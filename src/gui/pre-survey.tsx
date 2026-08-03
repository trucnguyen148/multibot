import React from 'react';
import { StageConfig } from '../utils';
import { Box, Button, Divider, Paper, Slider, Stack, Typography } from '@mui/material';

interface PreSurveyProps {
  stage: StageConfig;
  preSurvey: Record<string, any>;
  handleSliderChange: (surveyType: 'pre' | 'post', key: string, value: number | number[]) => void;
  handlePreSurveySubmit: (event: React.FormEvent<HTMLFormElement>) => void;
}

const ddiItems = [
  'When I feel upset, I usually confide in my friends.',
  'I prefer not to talk about my problems.',
  'When something unpleasant happens to me, I often look for someone to talk to.',
  "I typically don't discuss things that upset me.",
  'When I am depressed or sad, I tend to keep those feelings to myself.',
  'I try to find people to talk with about my problems.',
  'It is easy for me to tell others when I am unhappy.',
  'If I have a bad day, the last thing I want to do is talk about it.',
  'I rarely look for people to talk with when I am experiencing a problem.',
  'When I am in distress, I usually seek out someone to talk to.',
  'I am willing to tell others my distressing thoughts.',
  'I usually keep my troubles to myself.',
];

const ssrphItems = [
  'Seeing a professional or counselor for work-related stress makes a person look weak.',
  'I would feel embarrassed if my peers knew I was seeking help for academic or professional burnout.',
  'People tend to think less of someone who admits they cannot handle their workload.',
  'Seeking support for professional struggles can negatively impact how supervisors or professors view your competence.',
  'It is better to hide feelings of professional overwhelm than to risk being judged by others.',
];

const aiasItems = [
  'I see artificial intelligence as a threat.',
  'I think artificial intelligence reduces human-to-human communication.',
  'I am concerned that artificial intelligence will replace human labor.',
  'I believe artificial intelligence destroys creativity.',
];

export const PreSurvey: React.FC<PreSurveyProps> = ({
  stage,
  preSurvey,
  handleSliderChange,
  handlePreSurveySubmit,
}) => {
  return (
    <Paper elevation={2} sx={{ p: 4, borderRadius: 2 }}>
      <Typography variant="h4" gutterBottom>
        {stage.title}
      </Typography>
      <Box sx={{ bgcolor: 'info.light', p: 2, borderRadius: 1, mb: 4 }}>
        <Typography variant="body2" sx={{ color: 'info.contrastText' }}>
          Please answer the following questions honestly before entering the chat environment.
        </Typography>
      </Box>

      <form onSubmit={handlePreSurveySubmit}>
        <Stack spacing={5}>
          {/* DDI Section */}
          <Box>
            <Typography variant="h6" sx={{ color: 'primary.main', mb: 1 }}>
              Distress Disclosure Index (DDI)
            </Typography>
            <Typography variant="body2" sx={{ color: 'text.secondary', mb: 3 }}>
              1 = Strongly Disagree, 2 = Disagree, 3 = Neutral, 4 = Agree, 5 = Strongly Agree
            </Typography>
            <Stack spacing={3}>
              {ddiItems.map((itemText, i) => {
                const key = `DDI_${i + 1}`;
                const isAnswered = preSurvey[key] !== undefined;
                return (
                  <Box key={key}>
                    <Typography variant="body1">
                      {i + 1}. {itemText}
                    </Typography>
                    <Box sx={{ display: 'flex', alignItems: 'center', gap: 3, mt: 1 }}>
                      <Slider
                        value={(preSurvey[key] as number) || 3}
                        min={1}
                        max={5}
                        step={1}
                        marks
                        onChange={(_, val) => handleSliderChange('pre', key, val)}
                        onChangeCommitted={(_, val) => handleSliderChange('pre', key, val)}
                        sx={{
                          color: isAnswered ? 'primary.main' : 'grey.400',
                          '& .MuiSlider-thumb': {
                            display: isAnswered ? 'flex' : 'none',
                          },
                        }}
                      />
                      <Typography
                        variant="body2"
                        sx={{
                          color: isAnswered ? 'success.main' : 'error.main',
                          minWidth: '95px',
                          fontWeight: 'bold',
                        }}
                      >
                        {isAnswered ? `Score: ${preSurvey[key]}` : 'Unanswered'}
                      </Typography>
                    </Box>
                  </Box>
                );
              })}
            </Stack>
          </Box>
          <Divider />

          {/* SSRPH Section */}
          <Box>
            <Typography variant="h6" sx={{ color: 'primary.main', mb: 1 }}>
              Adapted Stigma Scale for Receiving Psychological Help (SSRPH)
            </Typography>
            <Typography variant="body2" sx={{ color: 'text.secondary', mb: 3 }}>
              0 = Strongly Disagree, 3 = Strongly Agree
            </Typography>
            <Stack spacing={3}>
              {ssrphItems.map((itemText, i) => {
                const key = `SSRPH_${i + 1}`;
                const isAnswered = preSurvey[key] !== undefined;
                return (
                  <Box key={key}>
                    <Typography variant="body1">
                      {i + 1}. {itemText}
                    </Typography>
                    <Box sx={{ display: 'flex', alignItems: 'center', gap: 3, mt: 1 }}>
                      <Slider
                        value={(preSurvey[key] as number) || 0}
                        min={0}
                        max={3}
                        step={1}
                        marks
                        onChange={(_, val) => handleSliderChange('pre', key, val)}
                        onChangeCommitted={(_, val) => handleSliderChange('pre', key, val)}
                        sx={{
                          color: isAnswered ? 'primary.main' : 'grey.400',
                          '& .MuiSlider-thumb': {
                            display: isAnswered ? 'flex' : 'none',
                          },
                        }}
                      />
                      <Typography
                        variant="body2"
                        sx={{
                          color: isAnswered ? 'success.main' : 'error.main',
                          minWidth: '95px',
                          fontWeight: 'bold',
                        }}
                      >
                        {isAnswered ? `Score: ${preSurvey[key]}` : 'Unanswered'}
                      </Typography>
                    </Box>
                  </Box>
                );
              })}
            </Stack>
          </Box>
          <Divider />

          {/* AIAS Section */}
          <Box>
            <Typography variant="h6" sx={{ color: 'primary.main', mb: 1 }}>
              Artificial Intelligence Attitude Scale (AIAS)
            </Typography>
            <Typography variant="body2" sx={{ color: 'text.secondary', mb: 3 }}>
              1 = Strongly Disagree, 2 = Disagree, 3 = Neutral, 4 = Agree, 5 = Strongly Agree
            </Typography>
            <Stack spacing={3}>
              {aiasItems.map((itemText, i) => {
                const key = `AIAS_${i + 1}`;
                const isAnswered = preSurvey[key] !== undefined;
                return (
                  <Box key={key}>
                    <Typography variant="body1">
                      {i + 1}. {itemText}
                    </Typography>
                    <Box sx={{ display: 'flex', alignItems: 'center', gap: 3, mt: 1 }}>
                      <Slider
                        value={(preSurvey[key] as number) || 3}
                        min={1}
                        max={5}
                        step={1}
                        marks
                        onChange={(_, val) => handleSliderChange('pre', key, val)}
                        onChangeCommitted={(_, val) => handleSliderChange('pre', key, val)}
                        sx={{
                          color: isAnswered ? 'primary.main' : 'grey.400',
                          '& .MuiSlider-thumb': {
                            display: isAnswered ? 'flex' : 'none',
                          },
                        }}
                      />
                      <Typography
                        variant="body2"
                        sx={{
                          color: isAnswered ? 'success.main' : 'error.main',
                          minWidth: '95px',
                          fontWeight: 'bold',
                        }}
                      >
                        {isAnswered ? `Score: ${preSurvey[key]}` : 'Unanswered'}
                      </Typography>
                    </Box>
                  </Box>
                );
              })}
            </Stack>
          </Box>
          <Divider />

          {/* Baseline for the repeated comfort measure. Taken here because the
              participant has not met the bots yet, so it is the only reading
              uncontaminated by the condition. Same 1 to 7 scale and anchors as
              the in-chat check-ins and the post-survey Self_Comfort item, so all
              five points sit on one scale. */}
          <Box>
            <Typography variant="h6" sx={{ color: 'primary.main', mb: 1 }}>
              Before you start
            </Typography>
            <Typography variant="body1" sx={{ mb: 1 }}>
              You are about to join a short group chat with other members about professional and
              academic challenges. How comfortable do you expect to feel sharing your thoughts in
              that chat?
            </Typography>
            <Box sx={{ display: 'flex', alignItems: 'center', gap: 3, mt: 1 }}>
              <Slider
                value={(preSurvey['Self_Comfort_Pre'] as number) || 4}
                min={1}
                max={7}
                step={1}
                marks
                onChange={(_, val) => handleSliderChange('pre', 'Self_Comfort_Pre', val)}
                onChangeCommitted={(_, val) => handleSliderChange('pre', 'Self_Comfort_Pre', val)}
                sx={{
                  color: preSurvey['Self_Comfort_Pre'] !== undefined ? 'primary.main' : 'grey.400',
                  '& .MuiSlider-thumb': {
                    display: preSurvey['Self_Comfort_Pre'] !== undefined ? 'flex' : 'none',
                  },
                }}
              />
              <Typography
                variant="body2"
                sx={{
                  color:
                    preSurvey['Self_Comfort_Pre'] !== undefined ? 'success.main' : 'error.main',
                  minWidth: '95px',
                  fontWeight: 'bold',
                }}
              >
                {preSurvey['Self_Comfort_Pre'] !== undefined
                  ? `Score: ${preSurvey['Self_Comfort_Pre']}`
                  : 'Unanswered'}
              </Typography>
            </Box>
            <Typography variant="caption" sx={{ color: 'text.secondary' }}>
              1 = Not at all comfortable, 7 = Extremely comfortable
            </Typography>
          </Box>
          <Divider />

          <Button
            type="submit"
            variant="contained"
            size="large"
            disabled={Object.keys(preSurvey).length < 22}
          >
            {Object.keys(preSurvey).length < 22
              ? 'Please answer all questions'
              : 'Submit Pre-Survey'}
          </Button>
        </Stack>
      </form>
    </Paper>
  );
};
