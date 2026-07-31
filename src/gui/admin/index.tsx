import React, { useCallback, useEffect, useState } from 'react';
import {
  Alert,
  Box,
  Button,
  CircularProgress,
  Container,
  Paper,
  Snackbar,
  Stack,
  Tab,
  Tabs,
  TextField,
  Typography,
} from '@mui/material';
import { ExperimentData } from '../../utils';
import {
  AdminAuthError,
  AdminStats,
  PurgeScope,
  SessionDetail,
  SessionSummary,
  deleteSession,
  downloadExport,
  fetchSession,
  fetchSessions,
  fetchStats,
  purgeSessions,
} from './api';
import { StatsPanel } from './stats-panel';
import { SessionsPanel } from './sessions-panel';
import { SessionDetailDialog } from './session-detail';
import { DangerPanel } from './danger-panel';
import { TestLinksPanel } from './test-links-panel';

const SECRET_STORAGE_KEY = 'multibot-admin-secret';

// sessionStorage rather than localStorage, so closing the tab forgets the
// secret. A shared research laptop should not stay logged in.
const readStoredSecret = (): string => {
  try {
    return window.sessionStorage.getItem(SECRET_STORAGE_KEY) ?? '';
  } catch {
    return '';
  }
};

const storeSecret = (secret: string) => {
  try {
    if (secret) window.sessionStorage.setItem(SECRET_STORAGE_KEY, secret);
    else window.sessionStorage.removeItem(SECRET_STORAGE_KEY);
  } catch {
    // Private browsing with storage disabled. The secret still works for this
    // render; it just will not survive a reload.
  }
};

// /admin?secret=… is the convenient way in. The secret moves straight into
// sessionStorage and is stripped from the address bar, so it does not sit in
// browser history or get shoulder-read off the URL bar. It never travels to the
// API this way, since those calls use the X-Admin-Secret header.
const takeSecretFromUrl = (): string => {
  const params = new URLSearchParams(window.location.search);
  const secret = params.get('secret') ?? '';
  if (!secret) return '';
  params.delete('secret');
  const query = params.toString();
  window.history.replaceState({}, '', window.location.pathname + (query ? `?${query}` : ''));
  return secret;
};

const secretFromUrl = takeSecretFromUrl();
if (secretFromUrl) storeSecret(secretFromUrl);

interface AdminPageProps {
  data: ExperimentData;
  apiBaseUrl?: string;
}

export const AdminPage: React.FC<AdminPageProps> = ({ data, apiBaseUrl }) => {
  const [secret, setSecret] = useState<string>(secretFromUrl || readStoredSecret());
  const [typedSecret, setTypedSecret] = useState('');
  const [sessions, setSessions] = useState<SessionSummary[]>([]);
  const [stats, setStats] = useState<AdminStats | null>(null);
  const [loading, setLoading] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [notice, setNotice] = useState<string | null>(null);
  const [tab, setTab] = useState(0);

  const [detailOpen, setDetailOpen] = useState(false);
  const [detail, setDetail] = useState<SessionDetail | null>(null);
  const [detailLoading, setDetailLoading] = useState(false);
  const [detailError, setDetailError] = useState<string | null>(null);

  // A 401 anywhere means the secret is wrong or was rotated. Drop straight back
  // to the prompt rather than leaving stale data on screen.
  const handleFailure = useCallback((failure: unknown) => {
    if (failure instanceof AdminAuthError) {
      setSecret('');
      storeSecret('');
      setSessions([]);
      setStats(null);
      setError('That secret was rejected.');
      return;
    }
    setError(failure instanceof Error ? failure.message : String(failure));
  }, []);

  const refresh = useCallback(async () => {
    if (!secret) return;
    setLoading(true);
    setError(null);
    try {
      const [nextSessions, nextStats] = await Promise.all([
        fetchSessions(apiBaseUrl, secret),
        fetchStats(apiBaseUrl, secret),
      ]);
      setSessions(nextSessions);
      setStats(nextStats);
    } catch (failure) {
      handleFailure(failure);
    } finally {
      setLoading(false);
    }
  }, [apiBaseUrl, secret, handleFailure]);

  useEffect(() => {
    refresh();
  }, [refresh]);

  const inspect = async (id: string) => {
    setDetailOpen(true);
    setDetailLoading(true);
    setDetailError(null);
    setDetail(null);
    try {
      setDetail(await fetchSession(apiBaseUrl, secret, id));
    } catch (failure) {
      if (failure instanceof AdminAuthError) {
        setDetailOpen(false);
        handleFailure(failure);
        return;
      }
      setDetailError(failure instanceof Error ? failure.message : String(failure));
    } finally {
      setDetailLoading(false);
    }
  };

  const removeOne = async (session: SessionSummary) => {
    const label = session.prolific_id || session.display_name || session.user_id;
    const confirmed = window.confirm(
      `Permanently delete ${session.user_id}?\n\n` +
        `Participant: ${label}\nCondition: ${session.condition}\n` +
        `State: ${session.current_state}\n` +
        `${session.test_mode ? 'Flagged as a test walkthrough.' : 'This is NOT a test row.'}\n\n` +
        'There is no undo.'
    );
    if (!confirmed) return;

    setBusy(true);
    try {
      await deleteSession(apiBaseUrl, secret, session.user_id);
      setNotice(`Deleted ${session.user_id}.`);
      await refresh();
    } catch (failure) {
      handleFailure(failure);
    } finally {
      setBusy(false);
    }
  };

  const purge = async (scope: PurgeScope) => {
    setBusy(true);
    try {
      const result = await purgeSessions(apiBaseUrl, secret, scope);
      setNotice(`Deleted ${result.deleted} row(s).`);
      await refresh();
    } catch (failure) {
      handleFailure(failure);
    } finally {
      setBusy(false);
    }
  };

  const exportData = async () => {
    setBusy(true);
    try {
      await downloadExport(apiBaseUrl, secret);
      setNotice('Export downloaded.');
    } catch (failure) {
      handleFailure(failure);
    } finally {
      setBusy(false);
    }
  };

  if (!secret) {
    return (
      <Box sx={{ minHeight: '100vh', bgcolor: '#f8f9fa', py: 8 }}>
        <Container maxWidth="sm">
          <Paper elevation={2} sx={{ p: 4, borderRadius: 2 }}>
            <Typography variant="h5" gutterBottom>
              Study admin
            </Typography>
            <Typography variant="body2" sx={{ color: 'text.secondary', mb: 3 }}>
              This page is public, because the frontend serves every unknown path from the same
              bundle. It holds nothing until the secret is entered, since every figure on it comes
              from an endpoint that checks the secret on each request.
            </Typography>
            {error && (
              <Alert severity="error" sx={{ mb: 2 }}>
                {error}
              </Alert>
            )}
            <form
              onSubmit={(event) => {
                event.preventDefault();
                if (!typedSecret.trim()) return;
                storeSecret(typedSecret.trim());
                setSecret(typedSecret.trim());
                setTypedSecret('');
              }}
            >
              <Stack spacing={2}>
                <TextField
                  autoFocus
                  fullWidth
                  type="password"
                  label="Admin secret"
                  value={typedSecret}
                  onChange={(event) => setTypedSecret(event.target.value)}
                />
                <Button type="submit" variant="contained" disabled={!typedSecret.trim()}>
                  Unlock
                </Button>
              </Stack>
            </form>
            <Typography variant="caption" sx={{ color: 'text.secondary', display: 'block', mt: 3 }}>
              This is <code>EXPORT_SECRET</code>. <code>/admin?secret=…</code> also works; the value
              is moved into session storage and removed from the address bar on arrival.
            </Typography>
          </Paper>
        </Container>
      </Box>
    );
  }

  return (
    <Box sx={{ minHeight: '100vh', bgcolor: '#f8f9fa', pb: 8 }}>
      <Box
        component="header"
        sx={{ bgcolor: 'white', px: 3, py: 2, borderBottom: 1, borderColor: 'divider' }}
      >
        <Container maxWidth="lg">
          <Box
            sx={{
              display: 'flex',
              justifyContent: 'space-between',
              alignItems: 'center',
              gap: 2,
              flexWrap: 'wrap',
            }}
          >
            <Typography variant="h6" sx={{ fontWeight: 'bold' }}>
              Study admin
            </Typography>
            <Stack direction="row" spacing={2} sx={{ alignItems: 'center' }}>
              {loading && <CircularProgress size={20} />}
              <Button size="small" onClick={refresh} disabled={loading}>
                Refresh
              </Button>
              <Button
                size="small"
                color="inherit"
                onClick={() => {
                  storeSecret('');
                  setSecret('');
                }}
              >
                Lock
              </Button>
            </Stack>
          </Box>
        </Container>
      </Box>

      <Container maxWidth="lg" sx={{ pt: 3 }}>
        {error && (
          <Alert severity="error" sx={{ mb: 3 }}>
            {error}
          </Alert>
        )}

        <Tabs value={tab} onChange={(_, next) => setTab(next)} sx={{ mb: 3 }}>
          <Tab label="Overview" />
          <Tab label={`Sessions (${sessions.length})`} />
          <Tab label="Export and delete" />
          <Tab label="Walk the study" />
        </Tabs>

        {tab === 0 &&
          (stats ? (
            <StatsPanel stats={stats} />
          ) : (
            <Typography sx={{ color: 'text.secondary' }}>Loading statistics…</Typography>
          ))}

        {tab === 1 && (
          <SessionsPanel sessions={sessions} onInspect={inspect} onDelete={removeOne} busy={busy} />
        )}

        {tab === 2 && (
          <DangerPanel sessions={sessions} busy={busy} onExport={exportData} onPurge={purge} />
        )}

        {tab === 3 && <TestLinksPanel data={data} apiBaseUrl={apiBaseUrl} />}
      </Container>

      <SessionDetailDialog
        open={detailOpen}
        loading={detailLoading}
        error={detailError}
        session={detail}
        onClose={() => setDetailOpen(false)}
      />

      <Snackbar
        open={Boolean(notice)}
        autoHideDuration={4000}
        onClose={() => setNotice(null)}
        message={notice ?? ''}
      />
    </Box>
  );
};
