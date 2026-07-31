import React, { useEffect, useState } from 'react';
import {
  Alert,
  Box,
  Chip,
  Container,
  Divider,
  Link,
  Paper,
  Stack,
  Typography,
} from '@mui/material';
import { ExperimentData } from '../utils';

interface TestMenuProps {
  data: ExperimentData;
  apiBaseUrl?: string;
}

// Every bot that speaks in a condition, in first-appearance order, read off the
// scripts rather than hardcoded. Renaming a persona in data.json updates this
// page on its own.
const speakersIn = (stages: Record<string, { sender: string }[]>): string[] => {
  const seen: string[] = [];
  for (const scripts of Object.values(stages)) {
    for (const { sender } of scripts) {
      if (!seen.includes(sender)) seen.push(sender);
    }
  }
  return seen;
};

const turnsIn = (stages: Record<string, { sender: string }[]>): string =>
  Object.keys(stages)
    .sort()
    .map((key) => stages[key].length)
    .join(' / ');

type Health = 'checking' | 'up' | 'down';

export const TestMenu: React.FC<TestMenuProps> = ({ data, apiBaseUrl }) => {
  const [health, setHealth] = useState<Health>('checking');

  // Worth surfacing here more than anywhere else. The participant flow falls
  // back to local state on any backend failure and still walks through to a
  // completion code, so a run that looks fine in the browser proves nothing
  // about whether anything was saved.
  useEffect(() => {
    let cancelled = false;
    if (!apiBaseUrl) {
      setHealth('down');
      return;
    }
    fetch(`${apiBaseUrl}/health`)
      .then((response) => {
        if (!cancelled) setHealth(response.ok ? 'up' : 'down');
      })
      .catch(() => {
        if (!cancelled) setHealth('down');
      });
    return () => {
      cancelled = true;
    };
  }, [apiBaseUrl]);

  const conditions = Object.entries(data.conditions);

  return (
    <Box sx={{ minHeight: '100vh', bgcolor: '#f8f9fa', py: 5 }}>
      <Container maxWidth="md">
        <Paper elevation={2} sx={{ p: 4, borderRadius: 2 }}>
          <Typography variant="h4" gutterBottom>
            Researcher test menu
          </Typography>
          <Typography variant="body1" sx={{ color: 'text.secondary', mb: 3 }}>
            Not part of the study. These links open the participant flow in test mode with a chosen
            condition, so you do not have to reload until chance hands you the one you wanted.
          </Typography>

          <Alert severity={health === 'up' ? 'success' : health === 'down' ? 'error' : 'info'}>
            {health === 'checking' && 'Checking the backend…'}
            {health === 'up' && `Backend reachable at ${apiBaseUrl}`}
            {health === 'down' &&
              (apiBaseUrl
                ? `Backend unreachable at ${apiBaseUrl}. The study will still walk through end to ` +
                  'end using local fallback data and will save nothing.'
                : 'REACT_APP_BACKEND_URL was not set when this bundle was built. Nothing will be saved.')}
          </Alert>

          <Divider sx={{ my: 4 }} />

          <Typography variant="h6" gutterBottom color="primary">
            Test mode, fixed condition
          </Typography>
          <Typography variant="body2" sx={{ color: 'text.secondary', mb: 3 }}>
            Every field is pre-filled with the lowest value on its scale, the chat gains a “Skip
            chat” button, and the session is flagged <code>test_mode</code> at creation. Flagged
            rows never consume a participant slot and are excluded from the condition tally.
          </Typography>

          <Stack spacing={2}>
            {conditions.map(([key, condition]) => (
              <Paper key={key} variant="outlined" sx={{ p: 2, borderRadius: 2 }}>
                <Box
                  sx={{
                    display: 'flex',
                    alignItems: 'center',
                    justifyContent: 'space-between',
                    gap: 2,
                    flexWrap: 'wrap',
                  }}
                >
                  <Box>
                    <Link
                      href={`/?test=true&condition=${encodeURIComponent(key)}`}
                      sx={{ fontWeight: 'bold', fontSize: 18 }}
                    >
                      {condition.label}
                    </Link>
                    <Typography variant="body2" sx={{ color: 'text.secondary', mt: 0.5 }}>
                      {speakersIn(condition.stages).join(', ')} · {turnsIn(condition.stages)} bot
                      turns across stages 1 / 2 / 3
                    </Typography>
                  </Box>
                  <Chip label={key} size="small" />
                </Box>
              </Paper>
            ))}
          </Stack>

          <Divider sx={{ my: 4 }} />

          <Typography variant="h6" gutterBottom color="primary">
            Other entry points
          </Typography>
          <Stack spacing={1.5}>
            <Link href="/?test=true">Test mode, condition assigned at random</Link>
            <Link href="/">Participant view, exactly what Prolific sends people to</Link>
            <Link href="/?PROLIFIC_PID=TEST_PID&STUDY_ID=TEST_STUDY&SESSION_ID=TEST_SESSION">
              Participant view with the Prolific url parameters filled in
            </Link>
          </Stack>

          <Alert severity="warning" sx={{ mt: 4 }}>
            The participant view consumes a real condition slot and is counted once it completes.
            Use the test links unless you are specifically checking allocation.
          </Alert>

          <Alert severity="info" sx={{ mt: 2 }}>
            This page is not access controlled, and neither is <code>?test=true</code>. A
            participant who finds either one can reach a completion code in seconds. Those sessions
            are flagged, so they can be rejected on Prolific, which is a process control rather than
            a technical one.
          </Alert>
        </Paper>
      </Container>
    </Box>
  );
};
