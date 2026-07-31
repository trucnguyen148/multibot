import React, { useState } from 'react';
import { Alert, Box, Button, MenuItem, Paper, Stack, TextField, Typography } from '@mui/material';
import { PurgeScope, SessionSummary } from './api';

// The phrase has to be typed exactly. A dialog you can click through is not a
// speed bump, and this button destroys participant data with no undo.
const SCOPES: { value: PurgeScope; label: string; phrase: string; describe: string }[] = [
  {
    value: 'test_only',
    label: 'Test walkthroughs only',
    phrase: 'delete test rows',
    describe: 'Rows flagged test_mode at creation. The everyday operation, and the safe one.',
  },
  {
    value: 'incomplete',
    label: 'Real sessions that never finished',
    phrase: 'delete incomplete',
    describe:
      'Real rows short of STATE_COMPLETE. Leaves test rows alone, since clearing abandonment is a different request from clearing your own walkthroughs.',
  },
  {
    value: 'all',
    label: 'Everything',
    phrase: 'delete everything',
    describe: 'Every row in the database, completed real participants included.',
  },
];

// Mirrors purgeTargets in src/go/admin.go so the count shown is the count
// deleted. The server decides; this is only a preview.
const countFor = (sessions: SessionSummary[], scope: PurgeScope): number =>
  sessions.filter((session) => {
    if (scope === 'all') return true;
    if (scope === 'test_only') return session.test_mode;
    return session.current_state !== 'STATE_COMPLETE' && !session.test_mode;
  }).length;

interface DangerPanelProps {
  sessions: SessionSummary[];
  busy: boolean;
  onExport: () => void;
  onPurge: (scope: PurgeScope) => void;
}

export const DangerPanel: React.FC<DangerPanelProps> = ({ sessions, busy, onExport, onPurge }) => {
  const [scope, setScope] = useState<PurgeScope>('test_only');
  const [typed, setTyped] = useState('');

  const selected = SCOPES.find((entry) => entry.value === scope) as (typeof SCOPES)[number];
  const affected = countFor(sessions, scope);
  const armed = typed.trim().toLowerCase() === selected.phrase && affected > 0 && !busy;

  return (
    <Paper variant="outlined" sx={{ p: 3, borderRadius: 2, borderColor: 'error.light' }}>
      <Typography variant="h6" gutterBottom>
        Export and delete
      </Typography>

      {/* Deliberately above the delete controls, so taking a copy first is the
          path of least resistance. */}
      <Stack direction="row" spacing={2} sx={{ mb: 3, flexWrap: 'wrap', alignItems: 'center' }}>
        <Button variant="contained" onClick={onExport} disabled={busy}>
          Download full export
        </Button>
        <Typography variant="body2" sx={{ color: 'text.secondary' }}>
          Every row including transcripts, the same JSON <code>/api/export</code> returns.
        </Typography>
      </Stack>

      <Alert severity="warning" sx={{ mb: 2 }}>
        Deleting is permanent and there is no soft-delete flag. It also changes the condition tally,
        because removing a completed session frees a slot in that condition and the allocator will
        hand it out again. That is what you want before recruiting and a surprise mid-recruitment.
      </Alert>

      <Stack spacing={2}>
        <TextField
          select
          size="small"
          label="What to delete"
          value={scope}
          onChange={(event) => {
            setScope(event.target.value as PurgeScope);
            setTyped('');
          }}
          sx={{ maxWidth: 420 }}
        >
          {SCOPES.map((entry) => (
            <MenuItem key={entry.value} value={entry.value}>
              {entry.label}
            </MenuItem>
          ))}
        </TextField>

        <Typography variant="body2" sx={{ color: 'text.secondary' }}>
          {selected.describe}
        </Typography>

        <Box sx={{ display: 'flex', gap: 2, alignItems: 'center', flexWrap: 'wrap' }}>
          <TextField
            size="small"
            label={`Type “${selected.phrase}” to confirm`}
            value={typed}
            onChange={(event) => setTyped(event.target.value)}
            sx={{ minWidth: 320 }}
          />
          <Button
            variant="contained"
            color="error"
            disabled={!armed}
            onClick={() => {
              onPurge(scope);
              setTyped('');
            }}
          >
            Delete {affected} row{affected === 1 ? '' : 's'}
          </Button>
        </Box>

        {affected === 0 && (
          <Typography variant="body2" sx={{ color: 'text.secondary' }}>
            Nothing currently matches this scope.
          </Typography>
        )}
      </Stack>
    </Paper>
  );
};
