import React, { useMemo, useState } from 'react';
import {
  Box,
  Button,
  Chip,
  FormControlLabel,
  Paper,
  Stack,
  Switch,
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableRow,
  TextField,
  Typography,
} from '@mui/material';
import { SessionSummary } from './api';

const STATE_SHORT: Record<string, string> = {
  STATE_ONBOARDING: 'onboarding',
  STATE_PRE_SURVEY: 'pre-survey',
  STATE_INTERACTION: 'chat',
  STATE_POST_SURVEY: 'post-survey',
  STATE_COMPLETE: 'complete',
};

const formatTimestamp = (value: string): string => {
  const parsed = new Date(value);
  return Number.isNaN(parsed.getTime()) ? '—' : parsed.toLocaleString();
};

const formatDuration = (seconds: number): string => {
  if (!seconds || seconds < 0) return '—';
  const minutes = Math.floor(seconds / 60);
  return minutes >= 1 ? `${minutes}m` : `${Math.round(seconds)}s`;
};

interface SessionsPanelProps {
  sessions: SessionSummary[];
  onInspect: (id: string) => void;
  onDelete: (session: SessionSummary) => void;
  busy: boolean;
}

export const SessionsPanel: React.FC<SessionsPanelProps> = ({
  sessions,
  onInspect,
  onDelete,
  busy,
}) => {
  const [showTestRows, setShowTestRows] = useState(true);
  const [search, setSearch] = useState('');

  const visible = useMemo(() => {
    const needle = search.trim().toLowerCase();
    return sessions.filter((session) => {
      if (!showTestRows && session.test_mode) return false;
      if (!needle) return true;
      return [session.user_id, session.prolific_id, session.display_name, session.condition]
        .filter(Boolean)
        .some((field) => String(field).toLowerCase().includes(needle));
    });
  }, [sessions, showTestRows, search]);

  const testCount = sessions.filter((session) => session.test_mode).length;

  return (
    <Paper variant="outlined" sx={{ p: 3, borderRadius: 2 }}>
      <Box
        sx={{
          display: 'flex',
          justifyContent: 'space-between',
          alignItems: 'center',
          gap: 2,
          flexWrap: 'wrap',
          mb: 2,
        }}
      >
        <Box>
          <Typography variant="h6">Sessions</Typography>
          <Typography variant="body2" sx={{ color: 'text.secondary' }}>
            {visible.length} shown of {sessions.length}, {testCount} flagged as test walkthroughs.
          </Typography>
        </Box>
        <Stack direction="row" spacing={2} sx={{ alignItems: 'center' }}>
          <TextField
            size="small"
            label="Filter"
            placeholder="id, Prolific id, name, condition"
            value={search}
            onChange={(event) => setSearch(event.target.value)}
          />
          <FormControlLabel
            control={
              <Switch checked={showTestRows} onChange={(_, next) => setShowTestRows(next)} />
            }
            label="Show test rows"
          />
        </Stack>
      </Box>

      <Box sx={{ overflowX: 'auto' }}>
        <Table size="small">
          <TableHead>
            <TableRow>
              <TableCell>Created</TableCell>
              <TableCell>Prolific id</TableCell>
              <TableCell>Name</TableCell>
              <TableCell>Condition</TableCell>
              <TableCell>State</TableCell>
              <TableCell align="right">Turns</TableCell>
              <TableCell align="right">Words</TableCell>
              <TableCell align="right">Host gen / fb</TableCell>
              <TableCell align="right">Took</TableCell>
              <TableCell align="right">Actions</TableCell>
            </TableRow>
          </TableHead>
          <TableBody>
            {visible.map((session) => (
              <TableRow
                key={session.user_id}
                hover
                // Test rows are tinted rather than hidden, so a real participant
                // is never deleted while clearing test data.
                sx={session.test_mode ? { bgcolor: 'warning.light', opacity: 0.75 } : undefined}
              >
                <TableCell sx={{ whiteSpace: 'nowrap' }}>
                  {formatTimestamp(session.created_at)}
                </TableCell>
                <TableCell>
                  {session.test_mode ? (
                    <Chip label="TEST" size="small" color="warning" />
                  ) : (
                    <code>{session.prolific_id || '—'}</code>
                  )}
                </TableCell>
                <TableCell>{session.display_name || '—'}</TableCell>
                <TableCell>
                  <Chip label={session.condition} size="small" variant="outlined" />
                </TableCell>
                <TableCell>{STATE_SHORT[session.current_state] ?? session.current_state}</TableCell>
                <TableCell align="right">{session.participant_turns}</TableCell>
                <TableCell align="right">{session.participant_words}</TableCell>
                <TableCell align="right">
                  <Typography
                    variant="body2"
                    color={session.mirror_fallback > 0 ? 'warning.main' : 'text.primary'}
                  >
                    {session.mirror_generated} / {session.mirror_fallback}
                  </Typography>
                </TableCell>
                <TableCell align="right">{formatDuration(session.duration_seconds)}</TableCell>
                <TableCell align="right">
                  <Stack direction="row" spacing={1} sx={{ justifyContent: 'flex-end' }}>
                    <Button size="small" onClick={() => onInspect(session.user_id)}>
                      View
                    </Button>
                    <Button
                      size="small"
                      color="error"
                      disabled={busy}
                      onClick={() => onDelete(session)}
                    >
                      Delete
                    </Button>
                  </Stack>
                </TableCell>
              </TableRow>
            ))}
            {visible.length === 0 && (
              <TableRow>
                <TableCell colSpan={10} align="center" sx={{ color: 'text.secondary', py: 4 }}>
                  No sessions match.
                </TableCell>
              </TableRow>
            )}
          </TableBody>
        </Table>
      </Box>
    </Paper>
  );
};
