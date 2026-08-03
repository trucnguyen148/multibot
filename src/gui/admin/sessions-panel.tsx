import React, { useMemo, useState } from 'react';
import {
  Box,
  Button,
  Checkbox,
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
  onDeleteSelected: (selected: SessionSummary[]) => void;
  busy: boolean;
}

export const SessionsPanel: React.FC<SessionsPanelProps> = ({
  sessions,
  onInspect,
  onDelete,
  onDeleteSelected,
  busy,
}) => {
  const [showTestRows, setShowTestRows] = useState(true);
  const [search, setSearch] = useState('');
  const [selected, setSelected] = useState<string[]>([]);

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

  // Selection is kept as ids and always intersected with what is on screen, so
  // narrowing the filter can never leave a hidden row armed for deletion.
  const selectableIds = visible.map((session) => session.user_id);
  const selectedVisible = selected.filter((id) => selectableIds.includes(id));
  const allVisibleSelected =
    selectableIds.length > 0 && selectedVisible.length === selectableIds.length;

  const toggleOne = (id: string) =>
    setSelected((prev) => (prev.includes(id) ? prev.filter((held) => held !== id) : [...prev, id]));

  const toggleAllVisible = () => setSelected(allVisibleSelected ? [] : selectableIds);

  const selectedRows = sessions.filter((session) => selectedVisible.includes(session.user_id));
  const selectedRealRows = selectedRows.filter((session) => !session.test_mode).length;

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

      {selectedRows.length > 0 && (
        <Box
          sx={{
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'space-between',
            gap: 2,
            flexWrap: 'wrap',
            mb: 2,
            p: 1.5,
            borderRadius: 2,
            bgcolor: 'action.selected',
          }}
        >
          <Typography variant="body2">
            {selectedRows.length} selected
            {selectedRealRows > 0 && (
              <Typography component="span" variant="body2" sx={{ color: 'error.main', ml: 1 }}>
                including {selectedRealRows} row{selectedRealRows === 1 ? '' : 's'} not flagged as a
                test
              </Typography>
            )}
          </Typography>
          <Stack direction="row" spacing={1}>
            <Button size="small" onClick={() => setSelected([])}>
              Clear selection
            </Button>
            <Button
              size="small"
              variant="contained"
              color="error"
              disabled={busy}
              onClick={() => {
                onDeleteSelected(selectedRows);
                setSelected([]);
              }}
            >
              Delete {selectedRows.length} selected
            </Button>
          </Stack>
        </Box>
      )}

      <Box sx={{ overflowX: 'auto' }}>
        <Table size="small">
          <TableHead>
            <TableRow>
              <TableCell padding="checkbox">
                <Checkbox
                  checked={allVisibleSelected}
                  indeterminate={selectedVisible.length > 0 && !allVisibleSelected}
                  onChange={toggleAllVisible}
                  slotProps={{ input: { 'aria-label': 'Select every session shown' } }}
                />
              </TableCell>
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
                selected={selectedVisible.includes(session.user_id)}
                sx={session.test_mode ? { bgcolor: 'warning.light', opacity: 0.75 } : undefined}
              >
                <TableCell padding="checkbox">
                  <Checkbox
                    checked={selectedVisible.includes(session.user_id)}
                    onChange={() => toggleOne(session.user_id)}
                    slotProps={{ input: { 'aria-label': `Select ${session.user_id}` } }}
                  />
                </TableCell>
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
                <TableCell colSpan={11} align="center" sx={{ color: 'text.secondary', py: 4 }}>
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
