import React from 'react';
import { StageConfig } from '../utils';
import {
  Box,
  Button,
  Divider,
  Paper,
  FormControlLabel,
  Stack,
  Typography,
  TextField,
  Checkbox,
} from '@mui/material';

interface OnBoardingProps {
  stage: StageConfig;
  prolificId: string;
  setProlificId: (id: string) => void;
  prolificIdFromUrl?: boolean;
  displayName: string;
  setDisplayName: (name: string) => void;
  consentGiven: boolean;
  setConsentGiven: (consent: boolean) => void;
  handleOnboardingSubmit: (event: React.FormEvent<HTMLFormElement>) => void;
}

// Bot personas in data.json. A participant who picks one of these would make
// their own messages indistinguishable from a bot's in the transcript.
const RESERVED_NAMES = ['vieno', 'sam', 'charlie'];

export const OnBoarding: React.FC<OnBoardingProps> = ({
  stage,
  prolificId,
  setProlificId,
  prolificIdFromUrl = false,
  displayName,
  setDisplayName,
  consentGiven,
  setConsentGiven,
  handleOnboardingSubmit,
}) => {
  const trimmedName = displayName.trim();
  const nameIsReserved = RESERVED_NAMES.includes(trimmedName.toLowerCase());
  const nameTooLong = trimmedName.length > 20;
  const nameError = nameIsReserved
    ? 'That name is already used by someone else in the chat. Please choose another.'
    : nameTooLong
      ? 'Please keep this under 20 characters.'
      : '';
  return (
    <Paper elevation={2} sx={{ p: 4, borderRadius: 2 }}>
      <Typography variant="h4" gutterBottom>
        {stage.title}
      </Typography>
      <Typography variant="body1" sx={{ color: 'text.secondary', mb: 2 }}>
        Thank you for participating in this Human-Computer Interaction study. Please review the
        information below and provide your details to begin.
      </Typography>

      <Divider sx={{ my: 3 }} />

      <form onSubmit={handleOnboardingSubmit}>
        <Stack spacing={4}>
          <Box>
            <Typography variant="subtitle1" gutterBottom sx={{ fontWeight: 'bold' }}>
              1. Prolific ID
            </Typography>
            <TextField
              fullWidth
              variant="outlined"
              label="Enter your Prolific ID"
              value={prolificId}
              onChange={(e) => setProlificId(e.target.value)}
              // Read-only rather than disabled, so it stays legible and screen
              // readers still announce it.
              slotProps={{ htmlInput: { readOnly: prolificIdFromUrl } }}
              helperText={
                prolificIdFromUrl ? 'Filled in automatically from your Prolific link.' : ''
              }
              required
            />
          </Box>
          <Box>
            <Typography variant="subtitle1" gutterBottom sx={{ fontWeight: 'bold' }}>
              2. Your name in the chat
            </Typography>
            <Typography variant="body2" sx={{ color: 'text.secondary', mb: 2 }}>
              This is how the other chat members will address you. A first name or a nickname is
              fine, and you do not have to use your real name.
            </Typography>
            <TextField
              fullWidth
              variant="outlined"
              label="First name or nickname"
              value={displayName}
              onChange={(e) => setDisplayName(e.target.value)}
              error={Boolean(nameError)}
              helperText={nameError}
              slotProps={{ htmlInput: { maxLength: 30 } }}
              required
            />
          </Box>

          <Box>
            <Typography variant="subtitle1" gutterBottom sx={{ fontWeight: 'bold' }}>
              3. Informed Consent
            </Typography>
            <Paper
              variant="outlined"
              sx={{ p: 2, bgcolor: '#f5f5f5', maxHeight: 150, overflow: 'auto', mb: 2 }}
            >
              <Typography variant="body2">
                By checking the box below, you acknowledge that you have read the study information,
                you are over 18 years of age, and you consent to your anonymized data being used for
                academic research purposes. You may withdraw at any time.
              </Typography>
            </Paper>
            <FormControlLabel
              control={
                <Checkbox
                  checked={consentGiven}
                  onChange={(e) => setConsentGiven(e.target.checked)}
                />
              }
              label="I agree to participate in this study."
            />
          </Box>

          <Button
            type="submit"
            variant="contained"
            size="large"
            disabled={!prolificId.trim() || !trimmedName || Boolean(nameError) || !consentGiven}
          >
            Start Pre-Survey
          </Button>
        </Stack>
      </form>
    </Paper>
  );
};
