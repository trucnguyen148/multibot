// The client half of /api/admin. Every call carries the secret in a header
// rather than the query string. That is the whole reason the export below is a
// fetch plus a blob download instead of a plain link, since an <a href> cannot
// set headers and would put the secret back in the URL.

export interface SessionSummary {
  user_id: string;
  condition: string;
  current_state: string;
  prolific_id?: string;
  display_name?: string;
  test_mode: boolean;
  created_at: string;
  updated_at?: string;
  duration_seconds: number;
  turns: number;
  participant_turns: number;
  participant_words: number;
  mirror_generated: number;
  mirror_fallback: number;
  has_post_survey: boolean;
}

export interface ConditionStat {
  condition: string;
  completed: number;
  in_progress: number;
  abandoned_at_intake: number;
  median_words_by_stage: Record<string, number>;
}

export interface AdminStats {
  target_per_condition: number;
  conditions: ConditionStat[];
  drop_off_by_state: Record<string, number>;
  total_sessions: number;
  test_sessions: number;
  mirror_generated: number;
  mirror_fallback: number;
  recent_mirror_generated: number;
  recent_mirror_fallback: number;
  recent_session_count: number;
  empty_transcripts: number;
  median_completion_seconds: number;
  mirror_cost: MirrorCostStats;
}

// Inference spend, recorded per call in the mirror_usage table. The paper has to
// state this, and an OpenRouter dashboard total cannot be split by participant or
// by condition after the fact.
export interface MirrorCostStats {
  calls: number;
  failed_calls: number;
  empty_inputs: number;
  sessions_billed: number;
  prompt_tokens: number;
  completion_tokens: number;
  cost_total: number;
  cost_per_session: number;
  cost_by_condition: Record<string, number>;
  model: string;
}

export interface TranscriptMessage {
  sender: string;
  text: string;
  stage: string;
  timestamp?: string;
  isUser?: boolean;
  mirror?: string;
}

export interface SessionDetail {
  user_id: string;
  condition: string;
  current_state: string;
  pre_survey_data?: Record<string, unknown>;
  chat_transcript?: TranscriptMessage[];
  post_survey_data?: Record<string, unknown>;
  stage_1_comfort_score?: number | null;
  stage_2_comfort_score?: number | null;
  created_at: string;
}

// Distinguished from any other failure so the page can drop back to the secret
// prompt instead of showing a generic error the researcher cannot act on.
export class AdminAuthError extends Error {}

export type PurgeScope = 'test_only' | 'incomplete' | 'all';

const request = async (
  apiBaseUrl: string | undefined,
  secret: string,
  path: string,
  init: RequestInit = {}
): Promise<Response> => {
  if (!apiBaseUrl) {
    throw new Error(
      'REACT_APP_BACKEND_URL was not set when this bundle was built, so there is no API to call.'
    );
  }
  const headers: Record<string, string> = { 'X-Admin-Secret': secret };
  if (init.body) headers['Content-Type'] = 'application/json';

  const response = await fetch(`${apiBaseUrl}${path}`, { ...init, headers });
  if (response.status === 401) throw new AdminAuthError('Unauthorized');
  if (!response.ok) throw new Error(`${response.status} ${response.statusText}`);
  return response;
};

const requestJSON = async <T>(
  apiBaseUrl: string | undefined,
  secret: string,
  path: string,
  init?: RequestInit
): Promise<T> => (await request(apiBaseUrl, secret, path, init)).json();

export const fetchSessions = (apiBaseUrl: string | undefined, secret: string) =>
  requestJSON<{ sessions: SessionSummary[] }>(apiBaseUrl, secret, '/api/admin/sessions').then(
    (payload) => payload.sessions ?? []
  );

export const fetchStats = (apiBaseUrl: string | undefined, secret: string) =>
  requestJSON<AdminStats>(apiBaseUrl, secret, '/api/admin/stats');

export const fetchSession = (apiBaseUrl: string | undefined, secret: string, id: string) =>
  requestJSON<SessionDetail>(apiBaseUrl, secret, `/api/admin/sessions/${encodeURIComponent(id)}`);

export const deleteSession = (apiBaseUrl: string | undefined, secret: string, id: string) =>
  request(apiBaseUrl, secret, `/api/admin/sessions/${encodeURIComponent(id)}`, {
    method: 'DELETE',
  }).then(() => undefined);

// One transaction on the server, so a hand-picked selection either goes or does
// not, rather than half-clearing if a row is missing.
export const deleteSessions = (apiBaseUrl: string | undefined, secret: string, ids: string[]) =>
  requestJSON<{ deleted: number }>(apiBaseUrl, secret, '/api/admin/sessions/delete', {
    method: 'POST',
    body: JSON.stringify({ ids }),
  });

export const purgeSessions = (apiBaseUrl: string | undefined, secret: string, scope: PurgeScope) =>
  requestJSON<{ deleted: number; scope: string }>(apiBaseUrl, secret, '/api/admin/purge', {
    method: 'POST',
    body: JSON.stringify({ scope }),
  });

// Downloads through fetch so the secret rides in a header. The filename carries
// a timestamp, since the usual reason to export twice is to keep a copy from
// before a delete and one from after.
export const downloadExport = async (apiBaseUrl: string | undefined, secret: string) => {
  const response = await request(apiBaseUrl, secret, '/api/export');
  const blob = await response.blob();
  const url = URL.createObjectURL(blob);
  const link = document.createElement('a');
  link.href = url;
  link.download = `study_data_${new Date().toISOString().replace(/[:.]/g, '-')}.json`;
  document.body.appendChild(link);
  link.click();
  document.body.removeChild(link);
  URL.revokeObjectURL(url);
};
