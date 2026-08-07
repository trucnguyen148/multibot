import React, { useState, useEffect, useRef } from 'react';
import { Box, Paper, Typography, TextField, IconButton, Button, Avatar, Fade } from '@mui/material';
import SendIcon from '@mui/icons-material/Send';
import SmartToyIcon from '@mui/icons-material/SmartToy';
import PersonIcon from '@mui/icons-material/Person';
import AssessmentIcon from '@mui/icons-material/Assessment';

// Bot messages are delayed in proportion to their length, so the pacing reads as
// someone typing rather than pasting. 350ms per word is roughly 170 words per
// minute, fast for a person and still believable. The previous 200ms was about
// 300 wpm, four times a realistic rate, and was the most obvious tell that the
// other chat members were scripted.
//
// The cap stops a long message from stalling the session. Anything past roughly
// ten seconds of "typing..." reads as a stall rather than as care.
const MS_PER_WORD = 350;
const BASE_DELAY_MS = 1000;
const MAX_DELAY_MS = 10000;

// Distinct avatar colors per bot so participants can tell speakers apart at a
// glance. Falls back to the old neutral grey for any sender not listed here.
const BOT_AVATAR_COLORS: Record<string, string> = {
  Vieno: '#5C6BC0',
  Sam: '#43A047',
  Charlie: '#FB8C00',
};
const DEFAULT_BOT_AVATAR_COLOR = 'grey.500';

// Participant turns in every stage, for every participant, in every condition:
// the answer to the stage's question, then one optional chance to add more.
// Mirrors mirrorTurnsPerStage in mirror.go. Both sides compute the same thing
// from the same turn index, so the structure holds even when the server is
// unreachable and no reply comes back at all.
const MIRROR_TURNS_PER_STAGE = 2;

const typingDelayFor = (text: string): number => {
  const wordCount = text.split(/\s+/).filter((word) => word.length > 0).length;
  return Math.min(wordCount * MS_PER_WORD + BASE_DELAY_MS, MAX_DELAY_MS);
};

interface BotScript {
  sender: string;
  text: string;
  tag?: string;
}

interface ChatMessage {
  id: string;
  sender: string;
  text: string;
  timestamp: string;
  stage: string;
  // Set explicitly rather than inferred by comparing sender to userName, so a
  // participant who calls themselves Sam or Vieno still renders correctly.
  isUser?: boolean;
  isAssessment?: boolean;
  assessmentScore?: number;
  // Present only on a host turn produced at runtime. Lets the export tell
  // generated text from script, a real acknowledgement from a timed-out one,
  // and a stage the participant closed by declining the invitation, when the
  // transcripts are coded.
  mirror?: 'generated' | 'fallback' | 'declined' | 'not-serious';
}

// What the host says when the backend cannot be reached at all. The backend has
// its own copy of these lines for when generation fails on its side; these only
// cover the network never getting there, and must stay identical to the
// constants in mirror.go or the two paths read differently in one transcript.
const MIRROR_FALLBACK = 'Thanks for sharing that.';
const MIRROR_INVITATION =
  'Take your time if there is anything else you would like to explore about this, or let me know when you are ready to continue.';
const MIRROR_DECLINE_ACK = 'That is completely fine, thank you.';

interface MirrorReply {
  text: string;
  mirror: 'generated' | 'fallback' | 'declined' | 'not-serious';
}

// What playMirror sends as `history`: the conversation so far, oldest first,
// across every stage played to this point (not just the current one), so the
// host answers with the whole conversation in view rather than being
// re-introduced to it every turn. Assessment check-ins are not conversation
// content and are left out.
interface MirrorHistoryTurn {
  sender: string;
  text: string;
  isUser: boolean;
}

interface ChatInterfaceProps {
  userName: string;
  conditionScripts: Record<string, BotScript[]>;
  testMode?: boolean;
  // Absent only when there is no server session at all, which is the offline
  // fallback path where nothing is being saved anyway. The stage structure is
  // the same either way, since it depends on the turn index rather than on
  // anything the server returns.
  requestMirror?: (
    userText: string,
    stage: string,
    history: MirrorHistoryTurn[],
    turnIndex: number,
    declined: boolean
  ) => Promise<MirrorReply>;
  onChatComplete: (
    transcript: ChatMessage[],
    stage1Score: number,
    stage2Score: number,
    stage3Score: number
  ) => void;
}

export const ChatInterface = ({
  userName,
  conditionScripts,
  testMode = false,
  requestMirror,
  onChatComplete,
}: ChatInterfaceProps) => {
  const [messages, setMessages] = useState<ChatMessage[]>([]);
  const [isTyping, setIsTyping] = useState<boolean>(false);
  const [typingBot, setTypingBot] = useState<string | null>(null);

  const [internalStage, setInternalStage] = useState<string>('STATE_CHAT_STAGE_1');
  // The repeated within-subjects measure, one per stage on the same 1 to 7 scale
  // as the post-survey Self_Comfort item, so the three in-session points and the
  // retrospective one can be read together. There was no stage 3 check-in until
  // 2026-08-03, which left the Late stage with no comfort measure at all.
  const [stage1Score, setStage1Score] = useState<number | null>(null);
  const [stage2Score, setStage2Score] = useState<number | null>(null);
  const [stage3Score, setStage3Score] = useState<number | null>(null);

  const [awaitingUser, setAwaitingUser] = useState<boolean>(false);
  // True only while the participant is sitting on the optional second turn, so
  // the decline button exists exactly where the invitation applies and nowhere
  // else. Held in state rather than read off turnsThisStageRef because a ref
  // does not re-render.
  const [canDecline, setCanDecline] = useState<boolean>(false);
  const [inputValue, setInputValue] = useState<string>('');

  const messagesEndRef = useRef<HTMLDivElement>(null);
  const stageStartedRef = useRef<Record<string, boolean>>({});
  // The submitted transcript is assembled from this rather than from `messages`,
  // which is stale inside the timeout that fires the closing line. Turns added
  // late (the host acknowledgement, the check-ins) would otherwise be dropped
  // from the saved transcript with nothing visible to show for it.
  const messagesRef = useRef<ChatMessage[]>([]);
  // Same reason as messagesRef. The stage 3 rating is set and used within one
  // event, so state has not committed by the time the closing timeout reads it.
  const latestStage3Ref = useRef<number>(0);
  // The participant's turn number within the current stage, 1 or 2, reset when
  // the stage changes. This is the only thing that decides where a stage ends,
  // on both sides: the server reads it to decide whether to append the
  // invitation, and this component reads it to decide whether the comfort
  // check-in follows.
  const turnsThisStageRef = useRef<number>(0);

  const scrollToBottom = () => {
    messagesEndRef.current?.scrollIntoView({ behavior: 'smooth' });
  };

  useEffect(() => {
    scrollToBottom();
    messagesRef.current = messages;
  }, [messages, isTyping]);

  useEffect(() => {
    const stageKey = internalStage;
    if (stageStartedRef.current[stageKey]) return;

    const scriptsToPlay = conditionScripts[stageKey];
    if (!scriptsToPlay || scriptsToPlay.length === 0) return;

    const startedStages = stageStartedRef.current;
    let isCancelled = false;
    startedStages[stageKey] = true;
    turnsThisStageRef.current = 0;
    setCanDecline(false);

    const playBotScripts = async () => {
      for (const script of scriptsToPlay) {
        if (isCancelled) return;

        setIsTyping(true);
        setTypingBot(script.sender);

        await new Promise<void>((resolve) => setTimeout(resolve, typingDelayFor(script.text)));

        if (isCancelled) return;

        setIsTyping(false);
        setMessages((prev) => [
          ...prev,
          {
            id: crypto.randomUUID(),
            sender: script.sender,
            text: script.text,
            timestamp: new Date().toISOString(),
            stage: stageKey,
          },
        ]);
      }

      if (!isCancelled) {
        setAwaitingUser(true);
      }
    };

    playBotScripts();

    return () => {
      isCancelled = true;
      startedStages[stageKey] = false;
    };
  }, [conditionScripts, internalStage]);

  // Plays the host's reply to what the participant just wrote, or to their
  // decline. Never throws: a failure here must not strand a participant
  // mid-study, and it must not change where the stage ends either.
  const playMirror = async (
    userText: string,
    stage: string,
    turnIndex: number,
    declined = false
  ): Promise<MirrorReply> => {
    // What the host says when the request never reaches the server. A decline
    // needs nothing generated, so offline it is not a degraded reply at all and
    // is marked as the decline it is rather than as a fallback.
    const offline: MirrorReply = declined
      ? { text: MIRROR_DECLINE_ACK, mirror: 'declined' }
      : {
          text:
            turnIndex < MIRROR_TURNS_PER_STAGE
              ? `${MIRROR_FALLBACK} ${MIRROR_INVITATION}`
              : MIRROR_FALLBACK,
          mirror: 'fallback',
        };

    const startedAt = Date.now();
    setIsTyping(true);
    setTypingBot('Vieno');

    // The whole conversation to this point, across every stage, so the host
    // answers with it in view rather than being re-introduced to it every
    // turn. Assessment check-ins are not conversation content.
    const history: MirrorHistoryTurn[] = messagesRef.current
      .filter((msg) => !msg.isAssessment)
      .map((msg) => ({ sender: msg.sender, text: msg.text, isUser: msg.isUser === true }));

    // A failure costs one acknowledgement and never a turn. Where the stage
    // ends is decided by the caller from the turn index alone, so a participant
    // who hits a network blip still gets the same structure as everyone else.
    let reply: MirrorReply = offline;
    if (requestMirror) {
      try {
        reply = await requestMirror(userText, stage, history, turnIndex, declined);
      } catch {
        // Keep the offline line: the network never reached the server at all.
        reply = offline;
      }
    }

    // Hold the reply until it has been "typed" at the same rate as every
    // scripted turn, counting the time the request already took. Without this
    // the host answers several times faster than she says anything else, which
    // is the most obvious tell that this one turn is machine generated.
    const remaining = typingDelayFor(reply.text) - (Date.now() - startedAt);
    if (remaining > 0) {
      await new Promise<void>((resolve) => setTimeout(resolve, remaining));
    }

    setIsTyping(false);
    setMessages((prev) => [
      ...prev,
      {
        id: crypto.randomUUID(),
        sender: 'Vieno',
        text: reply.text,
        timestamp: new Date().toISOString(),
        stage,
        mirror: reply.mirror,
      },
    ]);

    return reply;
  };

  const handleUserSubmit = async () => {
    if (!inputValue.trim()) return;

    const userMsg: ChatMessage = {
      id: crypto.randomUUID(),
      sender: userName || 'You',
      text: inputValue.trim(),
      timestamp: new Date().toISOString(),
      stage: internalStage,
      isUser: true,
    };

    setMessages((prev) => [...prev, userMsg]);
    setInputValue('');
    setAwaitingUser(false);
    setCanDecline(false);

    // Every stage runs the same two turns for every participant: their answer,
    // the host's acknowledgement carrying one invitation to add more, their
    // optional second turn, the host's closing acknowledgement. Nothing about
    // what was written, or about whether generation succeeded, changes that.
    //
    // A submission the host judges not to be a serious answer costs the turn
    // like any other. She says so once and the conversation carries on, so the
    // structure is identical for everyone regardless of what anyone writes.
    const turnIndex = turnsThisStageRef.current + 1;
    await playMirror(userMsg.text, internalStage, turnIndex);

    turnsThisStageRef.current = turnIndex;
    if (turnIndex >= MIRROR_TURNS_PER_STAGE) {
      askForComfort(internalStage);
    } else {
      setAwaitingUser(true);
      setCanDecline(true);
    }
  };

  // The other half of the invitation. Declining is a real answer, so it ends
  // the stage exactly where a typed second turn would, and is recorded as a
  // host turn marked "declined" with no participant message. That keeps a
  // decline a genuine zero in the word counts instead of the word "no".
  const handleDecline = async () => {
    setInputValue('');
    setAwaitingUser(false);
    setCanDecline(false);
    turnsThisStageRef.current = MIRROR_TURNS_PER_STAGE;
    await playMirror('', internalStage, MIRROR_TURNS_PER_STAGE, true);
    askForComfort(internalStage);
  };

  // The check-in is a normal transcript entry, so the rating and its position in
  // the conversation are both preserved in the export.
  const askForComfort = (stage: string) => {
    const assessmentMsg: ChatMessage = {
      id: crypto.randomUUID(),
      sender: 'System',
      text: 'Comfort check-in',
      timestamp: new Date().toISOString(),
      stage,
      isAssessment: true,
    };
    setTimeout(() => setMessages((prev) => [...prev, assessmentMsg]), 500);
  };

  const finishChat = () => {
    setInternalStage('STATE_CLOSING');
    setIsTyping(true);
    setTypingBot('Vieno');

    setTimeout(() => {
      setIsTyping(false);
      const closingMsg: ChatMessage = {
        id: crypto.randomUUID(),
        sender: 'Vieno',
        text: 'Thanks so much for sharing that. This concludes our session today!',
        timestamp: new Date().toISOString(),
        stage: 'STATE_CLOSING',
      };
      setMessages((prev) => [...prev, closingMsg]);
      setTimeout(() => {
        // The ref has caught up with the closing line by now, so appending it
        // again would store it twice. Matching on id keeps this correct whether
        // or not the render has landed.
        const latest = messagesRef.current;
        const finalTranscript = latest.some((message) => message.id === closingMsg.id)
          ? latest
          : [...latest, closingMsg];
        onChatComplete(
          finalTranscript,
          stage1Score || 0,
          stage2Score || 0,
          // Read off the click rather than the state, which has not committed
          // yet on the render that scheduled this.
          latestStage3Ref.current || 0
        );
      }, 2500);
    }, 1500);
  };

  const handleAssessmentSubmit = (msgId: string, score: number) => {
    setMessages((prev) =>
      prev.map((msg) => (msg.id === msgId ? { ...msg, assessmentScore: score } : msg))
    );

    if (internalStage === 'STATE_CHAT_STAGE_1') {
      setStage1Score(score);
      setInternalStage('STATE_CHAT_STAGE_2');
    } else if (internalStage === 'STATE_CHAT_STAGE_2') {
      setStage2Score(score);
      setInternalStage('STATE_CHAT_STAGE_3');
    } else if (internalStage === 'STATE_CHAT_STAGE_3') {
      setStage3Score(score);
      latestStage3Ref.current = score;
      finishChat();
    }
  };

  const isClosing =
    internalStage === 'STATE_CLOSING' && messages.length > 0 && messages.at(-1)?.sender === 'Vieno';

  return (
    <Fade in={!isClosing} timeout={1000}>
      <Paper
        elevation={3}
        sx={{
          display: 'flex',
          flexDirection: 'column',
          height: '80vh',
          maxWidth: '100vh',
          margin: '0 auto',
          overflow: 'hidden',
          borderRadius: 2,
        }}
      >
        <Box
          sx={{
            p: 2,
            bgcolor: 'primary.main',
            color: 'primary.contrastText',
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'space-between',
            gap: 2,
          }}
        >
          <Typography variant="h6">Group Chat</Typography>
          {testMode && (
            <Button
              variant="contained"
              color="warning"
              size="small"
              onClick={() =>
                onChatComplete(messages, stage1Score ?? 0, stage2Score ?? 0, stage3Score ?? 0)
              }
            >
              Skip chat
            </Button>
          )}
        </Box>

        <Box sx={{ flexGrow: 1, overflowY: 'auto', p: 3, bgcolor: '#f8f9fa' }}>
          {messages.map((msg) => {
            const isUser = msg.isUser === true;

            if (msg.isAssessment) {
              const isAnswered = msg.assessmentScore !== undefined;
              return (
                <Box key={msg.id} sx={{ display: 'flex', justifyContent: 'center', my: 3 }}>
                  <Paper
                    elevation={1}
                    sx={{
                      p: 2,
                      textAlign: 'center',
                      bgcolor: isAnswered ? '#e8f5e9' : 'white',
                      border: '1px solid',
                      borderColor: isAnswered ? '#a5d6a7' : 'divider',
                      borderRadius: 3,
                      maxWidth: '80%',
                    }}
                  >
                    <Box
                      sx={{
                        display: 'flex',
                        alignItems: 'center',
                        justifyContent: 'center',
                        gap: 1,
                        mb: 1,
                        color: isAnswered ? 'success.main' : 'text.secondary',
                      }}
                    >
                      <AssessmentIcon fontSize="small" />
                      <Typography variant="overline" sx={{ fontWeight: 'bold' }}>
                        {isAnswered ? 'Check-in Complete' : 'Quick Check-in'}
                      </Typography>
                    </Box>
                    <Typography variant="body2" gutterBottom sx={{ fontWeight: 500 }}>
                      Right now, how comfortable do you feel sharing your thoughts in this chat?
                    </Typography>
                    {isAnswered ? (
                      <Typography variant="h6" sx={{ color: 'success.main', mt: 1 }}>
                        Score: {msg.assessmentScore} / 7
                      </Typography>
                    ) : (
                      <Box
                        sx={{
                          display: 'flex',
                          justifyContent: 'center',
                          alignItems: 'center',
                          gap: 1,
                          mt: 2,
                        }}
                      >
                        <Typography variant="caption" sx={{ color: 'text.secondary' }}>
                          Not at all
                        </Typography>
                        {[1, 2, 3, 4, 5, 6, 7].map((score) => (
                          <Button
                            key={score}
                            variant="outlined"
                            onClick={() => handleAssessmentSubmit(msg.id, score)}
                            sx={{ minWidth: '40px', height: '40px', borderRadius: '50%' }}
                          >
                            {score}
                          </Button>
                        ))}
                        <Typography variant="caption" sx={{ color: 'text.secondary' }}>
                          Extremely
                        </Typography>
                      </Box>
                    )}
                  </Paper>
                </Box>
              );
            }

            return (
              <Box
                key={msg.id}
                sx={{
                  display: 'flex',
                  flexDirection: isUser ? 'row-reverse' : 'row',
                  alignItems: 'flex-start',
                  mb: 2,
                  gap: 1.5,
                }}
              >
                <Avatar
                  sx={{
                    bgcolor: isUser
                      ? 'primary.main'
                      : BOT_AVATAR_COLORS[msg.sender] || DEFAULT_BOT_AVATAR_COLOR,
                    width: 32,
                    height: 32,
                  }}
                >
                  {isUser ? <PersonIcon fontSize="small" /> : <SmartToyIcon fontSize="small" />}
                </Avatar>
                <Box sx={{ maxWidth: '75%' }}>
                  <Typography
                    variant="caption"
                    sx={{
                      color: 'text.secondary',
                      display: 'block',
                      mb: 0.5,
                      textAlign: isUser ? 'right' : 'left',
                    }}
                  >
                    {msg.sender}
                  </Typography>
                  <Paper
                    elevation={1}
                    sx={{
                      p: 1.5,
                      bgcolor: isUser ? 'primary.light' : 'white',
                      color: isUser ? 'primary.contrastText' : 'text.primary',
                      borderRadius: 2,
                      borderTopRightRadius: isUser ? 0 : 8,
                      borderTopLeftRadius: isUser ? 8 : 0,
                    }}
                  >
                    <Typography variant="body1" sx={{ whiteSpace: 'pre-wrap' }}>
                      {msg.text}
                    </Typography>
                  </Paper>
                </Box>
              </Box>
            );
          })}

          {isTyping && (
            <Fade in={isTyping}>
              <Box sx={{ display: 'flex', alignItems: 'center', gap: 1.5, mb: 2 }}>
                <Avatar
                  sx={{
                    bgcolor:
                      (typingBot && BOT_AVATAR_COLORS[typingBot]) || DEFAULT_BOT_AVATAR_COLOR,
                    width: 32,
                    height: 32,
                  }}
                >
                  <SmartToyIcon fontSize="small" />
                </Avatar>
                <Typography variant="body2" color="text.secondary" sx={{ fontStyle: 'italic' }}>
                  {typingBot} is typing...
                </Typography>
              </Box>
            </Fade>
          )}
          <div ref={messagesEndRef} />
        </Box>

        <Box sx={{ p: 2, bgcolor: 'background.paper', borderTop: 1, borderColor: 'divider' }}>
          <Box sx={{ display: 'flex', gap: 1 }}>
            <TextField
              fullWidth
              variant="outlined"
              placeholder={awaitingUser ? 'Type your message...' : 'Waiting...'}
              disabled={!awaitingUser}
              value={inputValue}
              onChange={(e) => setInputValue(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === 'Enter' && !e.shiftKey) {
                  e.preventDefault();
                  void handleUserSubmit();
                }
              }}
              size="small"
              multiline
              maxRows={3}
            />
            {/* The invitation to add more is optional, so there has to be a way
                to take it and add nothing. Without this the input box is the
                only way forward and a participant with nothing left to say has
                to invent something, which is a demand characteristic sitting
                directly on the disclosure measure. */}
            {canDecline && (
              <Button
                variant="text"
                size="small"
                onClick={() => void handleDecline()}
                disabled={!awaitingUser}
                sx={{
                  alignSelf: 'flex-end',
                  whiteSpace: 'nowrap',
                  // MUI shouts by default. This is a quiet way out of an
                  // optional question sitting next to the send button, so it
                  // should read as a link rather than compete with it.
                  textTransform: 'none',
                  color: 'text.secondary',
                  fontWeight: 400,
                  px: 1,
                  '&:hover': { bgcolor: 'transparent', color: 'text.primary' },
                }}
              >
                Nothing to add
              </Button>
            )}
            <IconButton
              color="primary"
              onClick={() => void handleUserSubmit()}
              disabled={!awaitingUser || !inputValue.trim()}
              sx={{ alignSelf: 'flex-end' }}
            >
              <SendIcon />
            </IconButton>
          </Box>
        </Box>
      </Paper>
    </Fade>
  );
};
