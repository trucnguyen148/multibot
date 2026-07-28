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
  completionCode?: string;
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

const personalizeScripts = (
  scripts: Record<string, BotScript[]>,
  userName: string
): Record<string, BotScript[]> => {
  const personalized: Record<string, BotScript[]> = {};
  const nameToUse = userName.trim() ? userName.trim() : 'there';

  for (const [stageKey, botScripts] of Object.entries(scripts)) {
    personalized[stageKey] = botScripts.map((script) => ({
      ...script,
      text: script.text.replace(/\[User's Name\]/g, nameToUse),
    }));
  }
  return personalized;
};

const initialPreSurvey: SurveyResponse = {};
const initialPostSurvey: SurveyResponse = {
  reflection: '',
};

// --- DYNAMIC API URL SETUP ---
const isLocal =
  window.location.hostname === 'localhost' || window.location.hostname === '127.0.0.1';

// TODO: When deploy Go backend to Railway, paste the live URL here
const API_BASE_URL = isLocal
  ? 'http://localhost:8080'
  : 'https://your-go-backend-railway-url.up.railway.app';

function App() {
  const [sessionId, setSessionId] = useState<string | null>(null);
  const [stage, setStage] = useState<StageConfig | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [isConnecting, setIsConnecting] = useState(false);

  // Onboarding State
  const [prolificId, setProlificId] = useState('');
  const [consentGiven, setConsentGiven] = useState(false);

  const [preSurvey, setPreSurvey] = useState<SurveyResponse>(initialPreSurvey);
  const [postSurvey, setPostSurvey] = useState<SurveyResponse>(initialPostSurvey);

  const buildFallbackStage = useCallback((currentState: string, condition = '1-1'): StageConfig => {
    const conditionData = experimentData.conditions[condition];

    // Define a clear, scalable mapping of state to its configuration
    const stageConfigMap: Record<string, { type: string; title: string }> = {
      STATE_ONBOARDING: { type: 'onboarding', title: 'Welcome to the Study' },
      STATE_PRE_SURVEY: { type: 'survey', title: 'Pre-interaction Survey' },
      STATE_INTERACTION: { type: 'chat', title: 'Peer Support Group Chat' },
      STATE_POST_SURVEY: { type: 'survey', title: 'Post-interaction Survey' },
      STATE_COMPLETE: { type: 'complete', title: 'Session Complete' },
    };

    // Extract the matched configuration, falling back to 'unknown' if it doesn't exist
    const { type, title } = stageConfigMap[currentState] || {
      type: 'unknown',
      title: 'Unknown Stage',
    };

    return {
      type,
      currentState,
      condition,
      title,
      allScripts: conditionData?.stages ?? {},
      completionCode: currentState === 'STATE_COMPLETE' ? 'DEMO_CODE_LOCAL' : undefined,
    };
  }, []);

  const initSession = useCallback(async () => {
    setLoading(true);
    try {
      // USING DYNAMIC URL
      const response = await fetch(`${API_BASE_URL}/api/session/init`, {
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
      setError(null);
    } finally {
      setLoading(false);
    }
  }, [buildFallbackStage]);

  const loadStage = async (sid: string) => {
    try {
      // USING DYNAMIC URL
      const response = await fetch(`${API_BASE_URL}/api/stage?sessionId=${sid}`);
      if (!response.ok) throw new Error('Backend unavailable');
      const payload = await response.json();
      setStage(payload.nextStage ?? payload);
    } catch {
      const condition = stage?.condition ?? '1-1';
      setStage(buildFallbackStage(stage?.currentState ?? 'STATE_ONBOARDING', condition));
    }
  };

  useEffect(() => {
    initSession();
  }, [initSession]);

  const submitStage = async (payload: Record<string, unknown>) => {
    if (!sessionId) return;

    try {
      // USING DYNAMIC URL
      const response = await fetch(`${API_BASE_URL}/api/submit`, {
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
    setIsConnecting(true);

    setTimeout(async () => {
      setIsConnecting(false);
      await submitStage({
        currentState: 'STATE_PRE_SURVEY',
        preSurveyData: preSurvey,
      });
    }, 1500);
  };

  const handlePostSurveySubmit = async (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault();

    await submitStage({
      currentState: 'STATE_POST_SURVEY',
      postSurveyData: postSurvey,
    });
  };

  // UPDATED: Added "| string" to the value type to support the reflection text box
  const handleSliderChange = (
    surveyType: 'pre' | 'post',
    key: string,
    value: number | number[] | string
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
                userName={prolificId}
                conditionScripts={personalizeScripts(stage.allScripts ?? {}, prolificId)}
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
          {stage.type === 'complete' && <CompleteScreen completionCode={stage.completionCode} />}
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
