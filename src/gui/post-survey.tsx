import React from 'react';
import { StageConfig } from '../utils';
import {
  Box,
  Button,
  Divider,
  FormControl,
  FormControlLabel,
  Paper,
  Radio,
  RadioGroup,
  Slider,
  Stack,
  TextField,
  Typography,
} from '@mui/material';

interface PostSurveyProps {
  stage: StageConfig;
  postSurvey: Record<string, any>;
  handleSliderChange: (
    surveyType: 'pre' | 'post',
    key: string,
    value: number | number[] | string
  ) => void;
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
  const isOfflineYes = postSurvey['offline_support'] === 'Yes';
  const requiredItemCount = isOfflineYes ? 16 : 15;
  const currentItemCount = Object.keys(postSurvey).filter((k) => postSurvey[k] !== '').length;
  const isComplete = currentItemCount >= requiredItemCount;

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
              1 = Not at all characteristic of me, 5 = Extremely characteristic of me
            </Typography>
            <Stack spacing={3}>
              {bfneItems.map((itemText, i) => {
                const key = `BFNE_${i + 1}`;
                const isAnswered = postSurvey[key] !== undefined;
                return (
                  <Box key={key}>
                    <Typography variant="body1">
                      {i + 1}. {itemText}
                    </Typography>
                    <Box sx={{ display: 'flex', alignItems: 'center', gap: 3, mt: 1 }}>
                      <Slider
                        value={(postSurvey[key] as number) || 3}
                        min={1}
                        max={5}
                        step={1}
                        marks
                        onChange={(_, val) => handleSliderChange('post', key, val)}
                        onChangeCommitted={(_, val) => handleSliderChange('post', key, val)}
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
                        {isAnswered ? `Score: ${postSurvey[key]}` : 'Unanswered'}
                      </Typography>
                    </Box>
                  </Box>
                );
              })}
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
                <Box sx={{ display: 'flex', alignItems: 'center', gap: 3, mt: 1 }}>
                  <Slider
                    value={(postSurvey[`Self_Comfort`] as number) || 4}
                    min={1}
                    max={7}
                    step={1}
                    marks
                    onChange={(_, val) => handleSliderChange('post', `Self_Comfort`, val)}
                    onChangeCommitted={(_, val) => handleSliderChange('post', `Self_Comfort`, val)}
                    sx={{
                      color: postSurvey[`Self_Comfort`] !== undefined ? 'primary.main' : 'grey.400',
                      '& .MuiSlider-thumb': {
                        display: postSurvey[`Self_Comfort`] !== undefined ? 'flex' : 'none',
                      },
                    }}
                  />
                  <Typography
                    variant="body2"
                    sx={{
                      color:
                        postSurvey[`Self_Comfort`] !== undefined ? 'success.main' : 'error.main',
                      minWidth: '95px',
                      fontWeight: 'bold',
                    }}
                  >
                    {postSurvey[`Self_Comfort`] !== undefined
                      ? `Score: ${postSurvey['Self_Comfort']}`
                      : 'Unanswered'}
                  </Typography>
                </Box>
              </Box>

              {/* Offline Support Section */}
              <Box>
                <Typography variant="h6" gutterBottom color="primary">
                  External Support
                </Typography>
                <FormControl component="fieldset">
                  <Typography variant="body1" sx={{ mb: 1 }}>
                    Have you previously discussed the professional or academic challenges you shared
                    today with another human?
                  </Typography>
                  <RadioGroup
                    row
                    value={postSurvey['offline_support'] || ''}
                    onChange={(e) => handleSliderChange('post', 'offline_support', e.target.value)}
                  >
                    <FormControlLabel value="Yes" control={<Radio />} label="Yes" />
                    <FormControlLabel value="No" control={<Radio />} label="No" />
                  </RadioGroup>
                </FormControl>

                {isOfflineYes && (
                  <TextField
                    fullWidth
                    sx={{ mt: 2 }}
                    variant="outlined"
                    label="Who did you tell?"
                    placeholder="e.g., A colleague, partner, therapist..."
                    value={(postSurvey['offline_support_details'] as string) || ''}
                    onChange={(e) =>
                      handleSliderChange('post', 'offline_support_details', e.target.value)
                    }
                    required
                  />
                )}
              </Box>
              <Divider />

              {/* Qualitative Reflection Section */}
              <Box>
                <Typography variant="h6" gutterBottom color="primary">
                  Open-ended Reflection
                </Typography>
                <Typography variant="body1" sx={{ mb: 2 }}>
                  What specifically about the chat environment made you feel more (or less)
                  comfortable sharing your experiences?
                </Typography>
                <TextField
                  fullWidth
                  multiline
                  rows={4}
                  variant="outlined"
                  placeholder="Type your reflection here..."
                  value={(postSurvey['reflection'] as string) || ''}
                  onChange={(e) => handleSliderChange('post', 'reflection', e.target.value)}
                  required
                />
              </Box>
            </Stack>
          </Box>

          <Button
            type="submit"
            variant="contained"
            size="large"
            color="success"
            disabled={!isComplete}
          >
            {isComplete ? 'Submit Session Data' : 'Please complete all fields'}
          </Button>
        </Stack>
      </form>
    </Paper>
  );
};
