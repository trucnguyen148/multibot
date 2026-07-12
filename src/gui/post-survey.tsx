import React from 'react';
import { StageConfig } from '../utils';
import { Box, Button, Divider, Paper, Slider, Stack, Typography } from '@mui/material';

interface PostSurveyProps {
  stage: StageConfig;
  postSurvey: Record<string, any>;
  handleSliderChange: (surveyType: 'pre' | 'post', key: string, value: number | number[]) => void;
  handlePostSurveySubmit: (event: React.FormEvent<HTMLFormElement>) => void;
}

const bfneItems = [
  'I worried about what the other chat members would think of me.',
  'I was unconcerned even if I thought the other members were forming an unfavorable impression of me. ',
  'I was afraid of the other members noticing my professional or academic shortcomings.',
  'I rarely worried about what kind of impression I was making on the others.',
  'I was afraid the other members would not approve of my responses.',
  'I was afraid that the other members would find fault with me.',
  'The opinion of other members of me did not bother me.',
  'While I was typing, I worried about what the others might be thinking about me.',
  'I was worried about what kind of impression I made in the chat.',
  'Knowing the others might be judging me had little effect on me.',
  'I felt I was too concerned with what the other chat members thought of me.',
  'I worried that I would say or type the wrong thing',
];

export const PostSurvey: React.FC<PostSurveyProps> = ({
  stage,
  postSurvey,
  handleSliderChange,
  handlePostSurveySubmit,
}) => {
  return (
    <Paper elevation={2} sx={{ p: 4, borderRadius: 2 }}>
      <Typography variant="h4" gutterBottom>
        {stage.title}
      </Typography>
      <Box sx={{ bgcolor: 'info.light', p: 2, borderRadius: 1, mb: 4 }}>
        <Typography variant="body2" sx={{ color: 'info.contrastText' }}>
          Thank you for completing the chat interaction. Please complete these final reflection
          questions.
        </Typography>
      </Box>

      <form onSubmit={handlePostSurveySubmit}>
        <Stack spacing={5}>
          {/* BFNE Section */}
          <Box>
            <Typography variant="h6" gutterBottom color="primary">
              Brief Fear of Negative Evaluation (State)
            </Typography>
            <Typography variant="body2" sx={{ color: 'text.secondary', mb: 3 }}>
              1 = Strongly Disagree, 2 = Disagree, 3 = Neutral, 4 = Agree, 5 = Strongly Agree
            </Typography>
            <Stack spacing={3}>
              {bfneItems.map((itemText, i) => (
                <Box key={`bfne-${i}`}>
                  <Typography variant="body1" gutterBottom>
                    {i + 1}. {itemText}
                  </Typography>
                  <Slider
                    value={(postSurvey[`BFNE_${i + 1}`] as number) || 3}
                    min={1}
                    max={5}
                    step={1}
                    marks
                    valueLabelDisplay="auto"
                    onChange={(_, val) => handleSliderChange('post', `BFNE_${i + 1}`, val)}
                  />
                </Box>
              ))}
            </Stack>
          </Box>
          <Divider />

          {/* Depth and Comfort Section */}
          <Box>
            <Typography variant="h6" gutterBottom color="primary">
              Self-Assessed Depth & Comfort
            </Typography>
            <Typography variant="body2" sx={{ color: 'text.secondary', mb: 3 }}>
              1 = Strongly Disagree, 7 = Strongly Agree
            </Typography>
            <Stack spacing={4}>
              <Box>
                <Typography variant="body1" gutterBottom>
                  I felt comfortable disclosing my true professional or academic challenges in this
                  chat.
                </Typography>
                <Slider
                  value={(postSurvey[`Self_Comfort`] as number) || 4}
                  min={1}
                  max={7}
                  step={1}
                  marks
                  valueLabelDisplay="auto"
                  onChange={(_, val) => handleSliderChange('post', `Self_Comfort`, val)}
                />
              </Box>
              <Box>
                <Typography variant="body1" gutterBottom>
                  I felt that I was able to share my thoughts deeply and honestly during the
                  interaction.
                </Typography>
                <Slider
                  value={(postSurvey[`Self_Depth`] as number) || 4}
                  min={1}
                  max={7}
                  step={1}
                  marks
                  valueLabelDisplay="auto"
                  onChange={(_, val) => handleSliderChange('post', `Self_Depth`, val)}
                />
              </Box>
            </Stack>
          </Box>

          <Button type="submit" variant="contained" size="large" color="success">
            Submit Session Data
          </Button>
        </Stack>
      </form>
    </Paper>
  );
};
