import React, { useCallback, useEffect, useState } from 'react';
import ReactDOM from 'react-dom/client';
import { ChatInterface } from './gui/chat-interface';
import fallbackExperimentData from './data.json';
import { Box, Container, Typography } from '@mui/material';
import { PreSurvey } from './gui/pre-survey';
import { OnBoarding } from './gui/onboarding';
import { PostSurvey } from './gui/post-survey';
import { CompleteScreen } from './gui/complete-screen';

interface BotScript {
  sender: string;
  text: string;
}

interface StageConfig {
  type: string;
  currentState: string;
  condition: string;
  title: string;
  allScripts?: Record<string, BotScript[]>;
}

interface SurveyResponse {
  [key: string]: string | number;
}

interface ExperimentData {
  conditions: Record<
    string,
    {
      label: string;
      stages: Record<string, BotScript[]>;
    }
  >;
}

const experimentData = fallbackExperimentData as ExperimentData;

function App() {
  const [sessionId, setSessionId] = useState<string | null>(null);
  const [stage, setStage] = useState<StageConfig | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [demoMode, setDemoMode] = useState(false);
  const [isConnecting, setIsConnecting] = useState(false);

  // Onboarding State
  const [prolificId, setProlificId] = useState('');
  const [consentGiven, setConsentGiven] = useState(false);

  const [preSurvey, setPreSurvey] = useState<SurveyResponse>({});
  const [postSurvey, setPostSurvey] = useState<SurveyResponse>({});

  const buildFallbackStage = useCallback((currentState: string, condition = '1-1'): StageConfig => {
    const conditionData = experimentData.conditions[condition];

    const type =
      currentState === 'STATE_ONBOARDING'
        ? 'onboarding'
        : currentState === 'STATE_PRE_SURVEY' || currentState === 'STATE_POST_SURVEY'
          ? 'survey'
          : currentState === 'STATE_COMPLETE'
            ? 'complete'
            : currentState === 'STATE_INTERACTION'
              ? 'chat'
              : 'unknown';

    const title =
      currentState === 'STATE_ONBOARDING'
        ? 'Welcome to the Study'
        : currentState === 'STATE_PRE_SURVEY'
          ? 'Pre-interaction Survey'
          : currentState === 'STATE_INTERACTION'
            ? 'Peer Support Group Chat'
            : currentState === 'STATE_POST_SURVEY'
              ? 'Post-interaction Survey'
              : currentState === 'STATE_COMPLETE'
                ? 'Session Complete'
                : 'Unknown Stage';

    return {
      type,
      currentState,
      condition,
      title,
      allScripts: conditionData?.stages ?? {},
    };
  }, []);

  const initSession = useCallback(async () => {
    setLoading(true);
    try {
      const response = await fetch('http://localhost:8080/api/session/init', {
        method: 'POST',
      });
      if (!response.ok) {
        throw new Error('Backend unavailable');
      }
      const payload = await response.json();
      setSessionId(payload.user_id ?? payload.sessionId ?? null);
      await loadStage(payload.user_id ?? payload.sessionId ?? '');
    } catch {
      const condition = ['1-1', '2-1', '3-1'][Math.floor(Math.random() * 3)];
      setSessionId(`demo-${Date.now()}`);
      // Start at the new ONBOARDING state
      setStage(buildFallbackStage('STATE_ONBOARDING', condition));
      setDemoMode(true);
      setError(null);
    } finally {
      setLoading(false);
    }
  }, [buildFallbackStage]);

  const loadStage = async (sid: string) => {
    try {
      const response = await fetch(`http://localhost:8080/api/stage?sessionId=${sid}`);
      if (!response.ok) throw new Error('Backend unavailable');
      const payload = await response.json();
      setStage(payload.nextStage ?? payload);
    } catch {
      const condition = stage?.condition ?? '1-1';
      setStage(buildFallbackStage(stage?.currentState ?? 'STATE_ONBOARDING', condition));
      setDemoMode(true);
    }
  };

  useEffect(() => {
    initSession();
  }, [initSession]);

  const submitStage = async (payload: Record<string, unknown>) => {
    if (!sessionId) return;

    try {
      const response = await fetch('http://localhost:8080/api/submit', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ sessionId, ...payload }),
      });
      const result = await response.json();
      if (result.nextStage) setStage(result.nextStage);
    } catch {
      const currentState =
        (payload.currentState as string) ?? stage?.currentState ?? 'STATE_ONBOARDING';

      // Updated routing map to include Onboarding
      const nextStateMap: Record<string, string> = {
        STATE_ONBOARDING: 'STATE_PRE_SURVEY',
        STATE_PRE_SURVEY: 'STATE_INTERACTION',
        STATE_INTERACTION: 'STATE_POST_SURVEY',
        STATE_POST_SURVEY: 'STATE_COMPLETE',
      };

      const nextState = nextStateMap[currentState] ?? 'STATE_COMPLETE';
      const currentCondition = stage?.condition ?? '1-1';
      setStage(buildFallbackStage(nextState, currentCondition));
      setDemoMode(true);
    }
  };

  const handleOnboardingSubmit = async (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (!prolificId || !consentGiven) return;
    await submitStage({
      currentState: 'STATE_ONBOARDING',
      prolificId,
    });
  };

  const handlePreSurveySubmit = async (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    setIsConnecting(true); // Trigger 3-second Ecological Validity Delay

    setTimeout(async () => {
      setIsConnecting(false);
      await submitStage({
        currentState: 'STATE_PRE_SURVEY',
        preSurveyData: preSurvey,
      });
    }, 3000);
  };

  const handlePostSurveySubmit = async (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    await submitStage({
      currentState: 'STATE_POST_SURVEY',
      postSurveyData: postSurvey,
    });
  };

  // Safe setter for MUI Slider
  const handleSliderChange = (
    surveyType: 'pre' | 'post',
    key: string,
    value: number | number[]
  ) => {
    const val = Array.isArray(value) ? value[0] : value;
    if (surveyType === 'pre') {
      setPreSurvey((prev) => ({ ...prev, [key]: val }));
    } else {
      setPostSurvey((prev) => ({ ...prev, [key]: val }));
    }
  };

  if (loading) return <Box sx={{ p: 4, textAlign: 'center' }}>Initializing the study session…</Box>;
  if (error) return <Box sx={{ p: 4, color: 'error.main' }}>{error}</Box>;
  if (!stage) return null;

  if (isConnecting) {
    return (
      <Box
        sx={{
          display: 'flex',
          flexDirection: 'column',
          alignItems: 'center',
          justifyContent: 'center',
          height: '100vh',
          bgcolor: '#f8f9fa',
        }}
      >
        <div className="h-12 w-12 animate-spin rounded-full border-b-2 border-t-2 border-blue-600"></div>
        <Typography variant="h6" sx={{ color: 'text.secondary', mt: 3 }}>
          Connecting to chat server...
        </Typography>
      </Box>
    );
  }

  return (
    <Box sx={{ height: '100vh', bgcolor: '#f8f9fa', display: 'flex', flexDirection: 'column' }}>
      <Box
        component="header"
        sx={{
          bgcolor: 'white',
          px: 3,
          py: 2,
          borderBottom: 1,
          borderColor: 'divider',
          boxShadow: 1,
        }}
      >
        <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
          <Box>
            <Typography variant="h6" sx={{ fontWeight: 'bold' }}>
              Multibot Research Prototype
            </Typography>
            <Typography variant="body2" sx={{ color: 'text.secondary' }}>
              Condition: {stage.condition}
            </Typography>
          </Box>
          {demoMode && (
            <Box
              sx={{
                px: 2,
                py: 0.5,
                bgcolor: 'warning.light',
                color: 'warning.dark',
                borderRadius: 4,
                border: 1,
                borderColor: 'warning.main',
              }}
            >
              <Typography variant="caption" sx={{ fontWeight: 'bold' }}>
                Demo Mode: Running Locally
              </Typography>
            </Box>
          )}
        </Box>
      </Box>

      <Box
        component="main"
        sx={{ flexGrow: 1, p: 3, overflow: 'auto', display: 'flex', justifyContent: 'center' }}
      >
        <Container maxWidth="md">
          {/* ONBOARDING SCREEN */}
          {stage.type === 'onboarding' && (
            <OnBoarding
              stage={stage}
              prolificId={prolificId}
              setProlificId={setProlificId}
              consentGiven={consentGiven}
              setConsentGiven={setConsentGiven}
              handleOnboardingSubmit={handleOnboardingSubmit}
            />
          )}

          {/* PRE-SURVEY SCREEN */}
          {stage.type === 'survey' && stage.currentState === 'STATE_PRE_SURVEY' && (
            <PreSurvey
              stage={stage}
              preSurvey={preSurvey}
              handleSliderChange={handleSliderChange}
              handlePreSurveySubmit={handlePreSurveySubmit}
            />
          )}

          {/* CHAT INTERFACE */}
          {stage.type === 'chat' && (
            <Box sx={{ height: '100%' }}>
              <ChatInterface
                conditionScripts={stage.allScripts ?? {}}
                onChatComplete={async (transcript, stage1Score, stage2Score) => {
                  await submitStage({
                    currentState: 'STATE_INTERACTION',
                    chatTranscript: transcript,
                    stage1ComfortScore: stage1Score,
                    stage2ComfortScore: stage2Score,
                  });
                }}
              />
            </Box>
          )}

          {/* POST-SURVEY SCREEN */}
          {stage.type === 'survey' && stage.currentState === 'STATE_POST_SURVEY' && (
            <PostSurvey
              stage={stage}
              postSurvey={postSurvey}
              handleSliderChange={handleSliderChange}
              handlePostSurveySubmit={handlePostSurveySubmit}
            />
          )}

          {/* COMPLETE SCREEN */}
          {stage.type === 'complete' && <CompleteScreen />}
        </Container>
      </Box>
    </Box>
  );
}

const root = ReactDOM.createRoot(document.getElementById('root') as HTMLElement);
root.render(
  <React.StrictMode>
    <App />
  </React.StrictMode>
);
