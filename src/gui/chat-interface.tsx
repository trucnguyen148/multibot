import React, { useState, useEffect, useRef } from 'react';
import { Box, Paper, Typography, TextField, IconButton, Button, Avatar, Fade } from '@mui/material';
import SendIcon from '@mui/icons-material/Send';
import SmartToyIcon from '@mui/icons-material/SmartToy';
import PersonIcon from '@mui/icons-material/Person';
import AssessmentIcon from '@mui/icons-material/Assessment';

interface BotScript {
  sender: string;
  text: string;
}

interface ChatMessage {
  id: string;
  sender: string;
  text: string;
  timestamp: string;
  stage: string;
  isAssessment?: boolean;
  assessmentScore?: number;
}

interface ChatInterfaceProps {
  conditionScripts: Record<string, BotScript[]>;
  onChatComplete: (transcript: ChatMessage[], stage1Score: number, stage2Score: number) => void;
}

export const ChatInterface = ({ conditionScripts, onChatComplete }: ChatInterfaceProps) => {
  // TOGGLE THIS TO TRUE TO RESTORE ASSESSMENTS LATER
  const ENABLE_ASSESSMENTS = false;

  const [messages, setMessages] = useState<ChatMessage[]>([]);
  const [isTyping, setIsTyping] = useState<boolean>(false);
  const [typingBot, setTypingBot] = useState<string | null>(null);

  const [internalStage, setInternalStage] = useState<string>('STATE_CHAT_STAGE_1');
  const [stage1Score, setStage1Score] = useState<number | null>(null);
  const [stage2Score, setStage2Score] = useState<number | null>(null);

  const [awaitingUser, setAwaitingUser] = useState<boolean>(false);
  const [inputValue, setInputValue] = useState<string>('');

  const messagesEndRef = useRef<HTMLDivElement>(null);
  const stageStartedRef = useRef<Record<string, boolean>>({});

  const scrollToBottom = () => {
    messagesEndRef.current?.scrollIntoView({ behavior: 'smooth' });
  };

  useEffect(() => {
    scrollToBottom();
  }, [messages, isTyping]);

  useEffect(() => {
    if (stageStartedRef.current[internalStage]) return;

    const scriptsToPlay = conditionScripts[internalStage];
    if (!scriptsToPlay || scriptsToPlay.length === 0) return;

    let isCancelled = false;
    stageStartedRef.current[internalStage] = true;

    const playBotScripts = async () => {
      for (const script of scriptsToPlay) {
        if (isCancelled) return;

        setIsTyping(true);
        setTypingBot(script.sender);

        const wordCount = script.text.split(/\s+/).filter((word) => word.length > 0).length;
        const delay = wordCount * 200 + 1000;

        await new Promise<void>((resolve) => setTimeout(resolve, delay));

        if (isCancelled) return;

        setIsTyping(false);
        setMessages((prev) => [
          ...prev,
          {
            id: crypto.randomUUID(),
            sender: script.sender,
            text: script.text,
            timestamp: new Date().toISOString(),
            stage: internalStage,
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
      stageStartedRef.current[internalStage] = false;
    };
  }, [conditionScripts, internalStage]);

  const handleUserSubmit = () => {
    if (!inputValue.trim()) return;

    const userMsg: ChatMessage = {
      id: crypto.randomUUID(),
      sender: 'User',
      text: inputValue.trim(),
      timestamp: new Date().toISOString(),
      stage: internalStage,
    };

    setMessages((prev) => [...prev, userMsg]);
    setInputValue('');
    setAwaitingUser(false);

    if (internalStage === 'STATE_CHAT_STAGE_1') {
      if (ENABLE_ASSESSMENTS) {
        const assessmentMsg: ChatMessage = {
          id: crypto.randomUUID(),
          sender: 'System',
          text: 'Assessment',
          timestamp: new Date().toISOString(),
          stage: internalStage,
          isAssessment: true,
        };
        setTimeout(() => setMessages((prev) => [...prev, assessmentMsg]), 500);
      } else {
        // Skip assessment and move directly to stage 2
        setInternalStage('STATE_CHAT_STAGE_2');
      }
    } else if (internalStage === 'STATE_CHAT_STAGE_2') {
      if (ENABLE_ASSESSMENTS) {
        const assessmentMsg: ChatMessage = {
          id: crypto.randomUUID(),
          sender: 'System',
          text: 'Assessment',
          timestamp: new Date().toISOString(),
          stage: internalStage,
          isAssessment: true,
        };
        setTimeout(() => setMessages((prev) => [...prev, assessmentMsg]), 500);
      } else {
        // Skip assessment and move directly to stage 3
        setInternalStage('STATE_CHAT_STAGE_3');
      }
    } else if (internalStage === 'STATE_CHAT_STAGE_3') {
      setInternalStage('STATE_CLOSING');

      setIsTyping(true);
      setTypingBot('Pete');

      setTimeout(() => {
        setIsTyping(false);
        const closingMsg: ChatMessage = {
          id: crypto.randomUUID(),
          sender: 'Pete',
          text: 'Thanks so much for sharing that. This concludes our session today!',
          timestamp: new Date().toISOString(),
          stage: 'STATE_CLOSING',
        };
        setMessages((prev) => [...prev, closingMsg]);

        setTimeout(() => {
          const finalTranscript = [...messages, userMsg, closingMsg];
          onChatComplete(finalTranscript, stage1Score || 0, stage2Score || 0);
        }, 2500);
      }, 1500);
    }
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
    }
  };

  const isClosing =
    internalStage === 'STATE_CLOSING' &&
    messages.length > 0 &&
    messages[messages.length - 1].sender === 'Pete';

  return (
    <Fade in={!isClosing} timeout={1000}>
      <Paper
        elevation={3}
        sx={{
          display: 'flex',
          flexDirection: 'column',
          height: '600px',
          maxWidth: '800px',
          margin: '0 auto',
          overflow: 'hidden',
          borderRadius: 2,
        }}
      >
        <Box sx={{ p: 2, bgcolor: 'primary.main', color: 'primary.contrastText' }}>
          <Typography variant="h6">Peer Support Chat</Typography>
        </Box>

        <Box sx={{ flexGrow: 1, overflowY: 'auto', p: 3, bgcolor: '#f8f9fa' }}>
          {messages.map((msg) => {
            const isUser = msg.sender === 'User';

            if (msg.isAssessment) {
              if (!ENABLE_ASSESSMENTS) return null;
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
                        Score: {msg.assessmentScore} / 5
                      </Typography>
                    ) : (
                      <Box sx={{ display: 'flex', justifyContent: 'center', gap: 1, mt: 2 }}>
                        {[1, 2, 3, 4, 5].map((score) => (
                          <Button
                            key={score}
                            variant="outlined"
                            onClick={() => handleAssessmentSubmit(msg.id, score)}
                            sx={{ minWidth: '40px', height: '40px', borderRadius: '50%' }}
                          >
                            {score}
                          </Button>
                        ))}
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
                  sx={{ bgcolor: isUser ? 'primary.main' : 'grey.500', width: 32, height: 32 }}
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
                <Avatar sx={{ bgcolor: 'grey.500', width: 32, height: 32 }}>
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
                  handleUserSubmit();
                }
              }}
              size="small"
              multiline
              maxRows={3}
            />
            <IconButton
              color="primary"
              onClick={handleUserSubmit}
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
