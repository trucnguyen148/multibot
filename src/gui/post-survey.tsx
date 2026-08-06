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

// Section headings are written for the participant, not for the codebook. The
// instrument is still the state-adapted BFNE and the response keys are
// unchanged; naming it, and calling it a "(State)" measure, only tells the
// participant what is being measured in them.
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

  // Checked by name rather than by counting filled keys. A count would let an
  // answered optional item stand in for a missing required one.
  const answered = (key: string): boolean => {
    const value = postSurvey[key];
    return value !== undefined && String(value).trim() !== '';
  };

  const requiredKeys = [
    ...bfneItems.map((_, i) => `BFNE_${i + 1}`),
    'Self_Comfort',
    'Self_Depth',
    'offline_support',
    'reflection',
    'held_back',
  ];

  const isComplete =
    requiredKeys.every(answered) && (!isOfflineYes || answered('offline_support_details'));

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
              How you felt during the chat
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

          {/* Comfort and depth. Self_Depth was specified in study-design.md and
              never built, so until now the section heading promised a depth
              measure the survey did not contain. It carries more weight than it
              did: with the stage length fixed at two turns, neither turn count
              nor word count says much about how far anyone actually went. */}
          <Box>
            <Typography variant="h6" gutterBottom color="primary">
              About the chat
            </Typography>
            <Typography variant="body2" sx={{ color: 'text.secondary', mb: 3 }}>
              1 = Strongly Disagree, 7 = Strongly Agree
            </Typography>
            <Stack spacing={4}>
              {[
                {
                  key: 'Self_Comfort',
                  text: 'I felt comfortable disclosing my true professional or academic challenges in this chat.',
                },
                {
                  key: 'Self_Depth',
                  text: 'I shared things about myself that I would normally keep private.',
                },
              ].map(({ key, text }) => (
                <Box key={key}>
                  <Typography variant="body1" gutterBottom>
                    {text}
                  </Typography>
                  <Box sx={{ display: 'flex', alignItems: 'center', gap: 3, mt: 1 }}>
                    <Slider
                      value={(postSurvey[key] as number) || 4}
                      min={1}
                      max={7}
                      step={1}
                      marks
                      onChange={(_, val) => handleSliderChange('post', key, val)}
                      onChangeCommitted={(_, val) => handleSliderChange('post', key, val)}
                      sx={{
                        color: postSurvey[key] !== undefined ? 'primary.main' : 'grey.400',
                        '& .MuiSlider-thumb': {
                          display: postSurvey[key] !== undefined ? 'flex' : 'none',
                        },
                      }}
                    />
                    <Typography
                      variant="body2"
                      sx={{
                        color: postSurvey[key] !== undefined ? 'success.main' : 'error.main',
                        minWidth: '95px',
                        fontWeight: 'bold',
                      }}
                    >
                      {postSurvey[key] !== undefined ? `Score: ${postSurvey[key]}` : 'Unanswered'}
                    </Typography>
                  </Box>
                </Box>
              ))}

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
                {/* Phrased so that "it made no difference" is a valid answer.
                    The earlier wording presupposed an effect, and because the
                    field is required, anyone who felt none had to invent one. */}
                <Typography variant="body1" sx={{ mb: 2 }}>
                  Did anything about the chat affect how comfortable you felt sharing your
                  experiences? If so, what? If it made no difference, please say so.
                </Typography>
                <TextField
                  fullWidth
                  multiline
                  rows={4}
                  variant="outlined"
                  placeholder="Type your answer here..."
                  value={(postSurvey['reflection'] as string) || ''}
                  onChange={(e) => handleSliderChange('post', 'reflection', e.target.value)}
                  required
                />

                {/* Restraint rather than disclosure. What someone withheld, and
                    why, is the other half of the story the transcripts tell, and
                    it is the item that still has something to say if the comfort
                    scores come back flat. Deliberately does not ask what the
                    thing was, only what made them hold back. */}
                <Typography variant="body1" sx={{ mt: 3, mb: 2 }}>
                  Was there anything you chose not to share in the chat? You do not need to say what
                  it was, only what made you hold back.
                </Typography>
                <TextField
                  fullWidth
                  multiline
                  rows={4}
                  variant="outlined"
                  placeholder="Type your answer here..."
                  value={(postSurvey['held_back'] as string) || ''}
                  onChange={(e) => handleSliderChange('post', 'held_back', e.target.value)}
                  required
                />

                <Typography variant="body1" sx={{ mt: 3, mb: 2 }}>
                  Did the responses from the other chat members influence what you chose to share?
                  If so, how?
                </Typography>
                <TextField
                  fullWidth
                  multiline
                  rows={4}
                  variant="outlined"
                  placeholder="Type your answer here..."
                  value={(postSurvey['reflection_influence'] as string) || ''}
                  onChange={(e) =>
                    handleSliderChange('post', 'reflection_influence', e.target.value)
                  }
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
            {isComplete ? 'Submit answers' : 'Please complete all fields'}
          </Button>
        </Stack>
      </form>
    </Paper>
  );
};
