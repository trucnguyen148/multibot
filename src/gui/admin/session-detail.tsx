import React from 'react';
import {
  Alert,
  Box,
  Chip,
  CircularProgress,
  Dialog,
  DialogContent,
  DialogTitle,
  Divider,
  Paper,
  Stack,
  Typography,
} from '@mui/material';
import { SessionDetail } from './api';

interface SessionDetailDialogProps {
  open: boolean;
  loading: boolean;
  error: string | null;
  session: SessionDetail | null;
  onClose: () => void;
}

// Colours the host turns by provenance. Coding a transcript without this
// distinction would treat generated text as scripted, and a timed-out fallback
// as a real acknowledgement.
const mirrorChip = (mirror?: string) => {
  if (mirror === 'generated') return <Chip size="small" color="info" label="generated" />;
  if (mirror === 'fallback') return <Chip size="small" color="warning" label="fallback" />;
  return <Chip size="small" variant="outlined" label="scripted" />;
};

const SurveyBlock: React.FC<{ title: string; data?: Record<string, unknown> }> = ({
  title,
  data,
}) => {
  const entries = Object.entries(data ?? {});
  if (entries.length === 0) return null;
  return (
    <Box>
      <Typography variant="subtitle2" gutterBottom>
        {title}
      </Typography>
      <Paper variant="outlined" sx={{ p: 2, borderRadius: 2 }}>
        <Box
          sx={{
            display: 'grid',
            gridTemplateColumns: 'repeat(auto-fill, minmax(180px, 1fr))',
            gap: 1,
          }}
        >
          {entries.map(([key, value]) => (
            <Typography key={key} variant="body2" sx={{ wordBreak: 'break-word' }}>
              <strong>{key}:</strong>{' '}
              {typeof value === 'object' ? JSON.stringify(value) : String(value)}
            </Typography>
          ))}
        </Box>
      </Paper>
    </Box>
  );
};

export const SessionDetailDialog: React.FC<SessionDetailDialogProps> = ({
  open,
  loading,
  error,
  session,
  onClose,
}) => (
  <Dialog open={open} onClose={onClose} maxWidth="md" fullWidth>
    <DialogTitle sx={{ pb: 1 }}>
      {session ? <code>{session.user_id}</code> : 'Session'}
      {session && (
        <Stack direction="row" spacing={1} sx={{ mt: 1 }}>
          <Chip size="small" label={session.condition} />
          <Chip size="small" variant="outlined" label={session.current_state} />
        </Stack>
      )}
    </DialogTitle>
    <DialogContent dividers>
      {loading && (
        <Box sx={{ display: 'flex', justifyContent: 'center', py: 4 }}>
          <CircularProgress />
        </Box>
      )}
      {error && <Alert severity="error">{error}</Alert>}

      {session && !loading && (
        <Stack spacing={3}>
          <SurveyBlock title="Pre-survey and identifiers" data={session.pre_survey_data} />
          <SurveyBlock title="Post-survey" data={session.post_survey_data} />

          <Box>
            <Typography variant="subtitle2" gutterBottom>
              Transcript ({session.chat_transcript?.length ?? 0} turns)
            </Typography>
            {(session.chat_transcript ?? []).length === 0 && (
              <Alert severity="warning">
                No transcript stored. If this session reached the post-survey, it ran on the
                demo-mode fallback and saved nothing.
              </Alert>
            )}
            <Stack spacing={1}>
              {(session.chat_transcript ?? []).map((message, index) => (
                <Paper
                  key={index}
                  variant="outlined"
                  sx={{
                    p: 1.5,
                    borderRadius: 2,
                    bgcolor: message.isUser ? 'primary.light' : 'background.paper',
                  }}
                >
                  <Box
                    sx={{
                      display: 'flex',
                      justifyContent: 'space-between',
                      alignItems: 'center',
                      gap: 2,
                      mb: 0.5,
                    }}
                  >
                    <Typography variant="caption" sx={{ fontWeight: 'bold' }}>
                      {message.sender}
                      {message.isUser ? ' (participant)' : ''}
                    </Typography>
                    <Stack direction="row" spacing={1} sx={{ alignItems: 'center' }}>
                      <Typography variant="caption" sx={{ color: 'text.secondary' }}>
                        {message.stage}
                      </Typography>
                      {!message.isUser && mirrorChip(message.mirror)}
                    </Stack>
                  </Box>
                  <Typography variant="body2" sx={{ whiteSpace: 'pre-wrap' }}>
                    {message.text}
                  </Typography>
                </Paper>
              ))}
            </Stack>
          </Box>

          <Divider />
          <Box>
            <Typography variant="subtitle2" gutterBottom>
              Raw row
            </Typography>
            <Paper variant="outlined" sx={{ p: 2, borderRadius: 2, overflowX: 'auto' }}>
              <Typography component="pre" variant="caption" sx={{ m: 0 }}>
                {JSON.stringify(session, null, 2)}
              </Typography>
            </Paper>
          </Box>
        </Stack>
      )}
    </DialogContent>
  </Dialog>
);
