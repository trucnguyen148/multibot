import React from 'react';
import {
  Alert,
  Box,
  Chip,
  Divider,
  LinearProgress,
  Paper,
  Stack,
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableRow,
  Typography,
} from '@mui/material';
import { AdminStats } from './api';

// The chat sub-states in the order they are played. Named here rather than read
// off the data, so a stage that stopped producing any words still shows as a
// column of zeroes instead of silently disappearing from the table.
const CHAT_STAGES = ['STATE_CHAT_STAGE_1', 'STATE_CHAT_STAGE_2', 'STATE_CHAT_STAGE_3'];

const STATE_LABELS: Record<string, string> = {
  STATE_ONBOARDING: 'Onboarding (page view)',
  STATE_PRE_SURVEY: 'Pre-survey',
  STATE_INTERACTION: 'Chat',
  STATE_POST_SURVEY: 'Post-survey',
  STATE_COMPLETE: 'Complete',
};

const formatDuration = (seconds: number): string => {
  if (!seconds) return '—';
  const minutes = Math.floor(seconds / 60);
  const remainder = Math.round(seconds % 60);
  return `${minutes}m ${remainder}s`;
};

interface StatsPanelProps {
  stats: AdminStats;
}

export const StatsPanel: React.FC<StatsPanelProps> = ({ stats }) => {
  const totalCompleted = stats.conditions.reduce((sum, stat) => sum + stat.completed, 0);
  const target = stats.target_per_condition * stats.conditions.length;

  const recentMirrorTurns = stats.recent_mirror_generated + stats.recent_mirror_fallback;
  const recentFallbackRate = recentMirrorTurns
    ? (stats.recent_mirror_fallback / recentMirrorTurns) * 100
    : 0;

  return (
    <Stack spacing={3}>
      <Paper variant="outlined" sx={{ p: 3, borderRadius: 2 }}>
        <Typography variant="h6" gutterBottom>
          Am I collecting the sample I need?
        </Typography>
        <Typography variant="body2" sx={{ color: 'text.secondary', mb: 2 }}>
          {totalCompleted} of {target} completions, {stats.target_per_condition} per condition. Test
          rows are excluded from every figure here and counted separately below.
        </Typography>

        <Table size="small">
          <TableHead>
            <TableRow>
              <TableCell>Condition</TableCell>
              <TableCell sx={{ minWidth: 200 }}>Completed</TableCell>
              <TableCell align="right">In progress</TableCell>
              <TableCell align="right">Left at intake</TableCell>
              <TableCell align="right">Median participant words, stage 1 / 2 / 3</TableCell>
            </TableRow>
          </TableHead>
          <TableBody>
            {stats.conditions.map((stat) => (
              <TableRow key={stat.condition}>
                <TableCell>
                  <Chip label={stat.condition} size="small" />
                </TableCell>
                <TableCell>
                  <Box sx={{ display: 'flex', alignItems: 'center', gap: 1.5 }}>
                    <LinearProgress
                      variant="determinate"
                      value={Math.min(100, (stat.completed / stats.target_per_condition) * 100)}
                      sx={{ flexGrow: 1, height: 8, borderRadius: 4 }}
                    />
                    <Typography variant="body2" sx={{ whiteSpace: 'nowrap' }}>
                      {stat.completed} / {stats.target_per_condition}
                    </Typography>
                  </Box>
                </TableCell>
                <TableCell align="right">{stat.in_progress}</TableCell>
                <TableCell align="right">{stat.abandoned_at_intake}</TableCell>
                <TableCell align="right">
                  {CHAT_STAGES.map((stage) => stat.median_words_by_stage[stage] ?? 0).join(' / ')}
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>

        <Divider sx={{ my: 3 }} />

        <Typography variant="subtitle2" gutterBottom>
          Where people stop
        </Typography>
        <Stack direction="row" spacing={1} useFlexGap sx={{ flexWrap: 'wrap' }}>
          {Object.keys(STATE_LABELS).map((state) => (
            <Chip
              key={state}
              size="small"
              variant="outlined"
              label={`${STATE_LABELS[state]}: ${stats.drop_off_by_state[state] ?? 0}`}
            />
          ))}
        </Stack>
        <Typography variant="caption" sx={{ color: 'text.secondary', display: 'block', mt: 1.5 }}>
          A row is written the instant the page loads, so anyone sitting at onboarding is a page
          view rather than a participant. The allocator ignores them for the same reason.
        </Typography>
      </Paper>

      <Paper variant="outlined" sx={{ p: 3, borderRadius: 2 }}>
        <Typography variant="h6" gutterBottom>
          Is the software behaving?
        </Typography>

        <Alert
          severity={
            recentFallbackRate > 20 ? 'error' : recentFallbackRate > 5 ? 'warning' : 'success'
          }
          sx={{ mb: 2 }}
        >
          <strong>Host acknowledgements, last {stats.recent_session_count} real sessions:</strong>{' '}
          {stats.recent_mirror_generated} generated, {stats.recent_mirror_fallback} fallback (
          {recentFallbackRate.toFixed(0)}%).{' '}
          {recentFallbackRate > 20
            ? 'Generation is failing. Pause recruitment on Prolific rather than collecting a different study.'
            : 'A sustained rise here means the OpenRouter path is failing and recruitment should pause.'}
        </Alert>

        <Stack direction="row" spacing={1} useFlexGap sx={{ mb: 2, flexWrap: 'wrap' }}>
          <Chip
            size="small"
            variant="outlined"
            label={`All time: ${stats.mirror_generated} generated / ${stats.mirror_fallback} fallback`}
          />
          <Chip
            size="small"
            variant="outlined"
            label={`Median time to completion: ${formatDuration(stats.median_completion_seconds)}`}
          />
          <Chip
            size="small"
            variant="outlined"
            label={`Rows in the database: ${stats.total_sessions}`}
          />
          <Chip
            size="small"
            variant="outlined"
            color={stats.test_sessions > 0 ? 'warning' : 'default'}
            label={`Test rows: ${stats.test_sessions}`}
          />
        </Stack>

        {stats.empty_transcripts > 0 && (
          <Alert severity="error">
            {stats.empty_transcripts} session(s) reached the post-survey with an empty transcript.
            That is the signature of the demo-mode fallback, where the participant walked the whole
            flow against local state and nothing was saved. Check <code>REACT_APP_BACKEND_URL</code>{' '}
            and the backend logs before recruiting further.
          </Alert>
        )}
      </Paper>
    </Stack>
  );
};
