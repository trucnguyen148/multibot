export interface BotScript {
  sender: string;
  text: string;
}

export interface StageConfig {
  type: string;
  currentState: string;
  condition: string;
  title: string;
  allScripts?: Record<string, BotScript[]>;
  completionCode?: string;
}

export interface SurveyResponse {
  [key: string]: string | number;
}

export interface ExperimentData {
  conditions: Record<
    string,
    {
      label: string;
      stages: Record<string, BotScript[]>;
    }
  >;
}
