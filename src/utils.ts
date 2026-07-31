export interface BotScript {
  sender: string;
  text: string;
  // Classifies the turn (open / peer-neutral / question / disclosure / ack).
  // The chat does not render it; it exists so src/go/scripts_test.go can check
  // the experimental invariants mechanically.
  tag?: string;
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
