import React, { useCallback, useEffect, useState } from 'react';
import ReactDOM from 'react-dom/client';
import { ChatInterface } from './gui/chat-interface';
import fallbackExperimentData from './data.json';
import { Box, Container, Typography } from '@mui/material';
import { PreSurvey } from './gui/pre-survey';
import { OnBoarding } from './gui/onboarding';
import { PostSurvey } from './gui/post-survey';
import { CompleteScreen } from './gui/complete-screen';
import { BotScript, ExperimentData, StageConfig, SurveyResponse } from './utils';

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

// REACT_APP_BACKEND_URL is baked in at build time, not read at runtime, so
// changing it on the host requires a rebuild rather than a restart.
const configuredBackendUrl = process.env.REACT_APP_BACKEND_URL?.replace(/\/+$/, '');

const API_BASE_URL = isLocal ? 'http://localhost:8080' : configuredBackendUrl;

if (!isLocal && !API_BASE_URL) {
  // Every request would resolve against "undefined/api/...", fail, and drop the
  // participant into the offline fallback below, which saves no data at all.
  console.error(
    'REACT_APP_BACKEND_URL was not set when this bundle was built. ' +
      'No responses will reach the backend.'
  );
}

function App() {
  const [sessionId, setSessionId] = useState<string | null>(null);
  const [stage, setStage] = useState<StageConfig | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [isConnecting, setIsConnecting] = useState(false);

  // Onboarding State
  const [prolificId, setProlificId] = useState('');
  const [displayName, setDisplayName] = useState('');
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

  const loadStage = useCallback(
    async (sid: string) => {
      try {
        const response = await fetch(`${API_BASE_URL}/api/stage?sessionId=${sid}`);
        if (!response.ok) throw new Error('Backend unavailable');
        const payload = await response.json();
        setStage(payload.nextStage ?? payload);
      } catch {
        setStage(buildFallbackStage('STATE_ONBOARDING', '1-1'));
      }
    },
    [buildFallbackStage]
  );

  const initSession = useCallback(async () => {
    setLoading(true);
    try {
      const response = await fetch(`${API_BASE_URL}/api/session/init`, {
        method: 'POST',
      });
      if (!response.ok) {
        throw new Error('Backend unavailable');
      }
      const payload = await response.json();
      const nextSessionId = payload.user_id ?? payload.sessionId ?? null;
      setSessionId(nextSessionId);
      await loadStage(nextSessionId ?? '');
    } catch {
      const condition = ['1-1', '2-1', '3-1'][Math.floor(Math.random() * 3)];
      setSessionId(`demo-${Date.now()}`);
      setStage(buildFallbackStage('STATE_ONBOARDING', condition));
      setError(null);
    } finally {
      setLoading(false);
    }
  }, [buildFallbackStage, loadStage]);

  useEffect(() => {
    initSession();
  }, [initSession]);

  const submitStage = async (payload: Record<string, unknown>) => {
    const currentState =
      (payload.currentState as string) ?? stage?.currentState ?? 'STATE_ONBOARDING';
    const currentCondition = stage?.condition ?? '1-1';
    const nextStateMap: Record<string, string> = {
      STATE_ONBOARDING: 'STATE_PRE_SURVEY',
      STATE_PRE_SURVEY: 'STATE_INTERACTION',
      STATE_INTERACTION: 'STATE_POST_SURVEY',
      STATE_POST_SURVEY: 'STATE_COMPLETE',
    };
    const nextState = nextStateMap[currentState] ?? 'STATE_COMPLETE';
    const fallbackStage = buildFallbackStage(nextState, currentCondition);

    const effectiveSessionId = sessionId ?? `demo-${Date.now()}`;
    if (!sessionId) {
      setSessionId(effectiveSessionId);
    }

    setStage(fallbackStage);

    try {
      const response = await fetch(`${API_BASE_URL}/api/submit`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ sessionId: effectiveSessionId, ...payload }),
      });
      if (!response.ok) throw new Error('Backend unavailable');
      const result = await response.json();

      // Without an explicit throw here a malformed reply would leave the
      // participant on a screen whose submit button silently does nothing.
      if (!result.nextStage) throw new Error('Response missing nextStage');
      setStage(result.nextStage);
    } catch {
      setStage(fallbackStage);
    }
  };

  const handleOnboardingSubmit = async (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (!prolificId.trim() || !displayName.trim() || !consentGiven) return;
    await submitStage({
      currentState: 'STATE_ONBOARDING',
      prolificId: prolificId.trim(),
      displayName: displayName.trim(),
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
              displayName={displayName}
              setDisplayName={setDisplayName}
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
                userName={displayName}
                conditionScripts={personalizeScripts(stage.allScripts ?? {}, displayName)}
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
