import React from 'react';
import { StageConfig } from '../utils';
import { Box, Button, Divider, Paper, Slider, Stack, Typography } from '@mui/material';

interface PostSurveyProps {
  stage: StageConfig;
  postSurvey: Record<string, any>;
  handleSliderChange: (surveyType: 'pre' | 'post', key: string, value: number | number[]) => void;
  handlePostSurveySubmit: (event: React.FormEvent<HTMLFormElement>) => void;
}

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
              1 = Not at all, 5 = Extremely
            </Typography>
            <Stack spacing={3}>
              {Array.from({ length: 12 }, (_, i) => (
                <Box key={`bfne-${i}`}>
                  <Typography variant="body1" gutterBottom>
                    Item {i + 1}
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
