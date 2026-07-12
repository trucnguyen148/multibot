import React from 'react';
import { Box, Paper, Typography } from '@mui/material';

interface CompleteScreenProps {}

export const CompleteScreen: React.FC<CompleteScreenProps> = ({}) => {
  return (
    <Paper elevation={2} sx={{ p: 6, textAlign: 'center', borderRadius: 2 }}>
      <Typography variant="h4" color="success.main" gutterBottom>
        Study Complete
      </Typography>
      <Typography variant="body1" sx={{ color: 'text.secondary', mb: 2 }}>
        Thank you for participating in this Human-Computer Interaction study. Please review the
        information below and provide your Prolific ID to begin.
      </Typography>
      <Box sx={{ bgcolor: '#e8f5e9', p: 3, borderRadius: 2, display: 'inline-block', mt: 2 }}>
        <Typography variant="overline" sx={{ color: 'success.dark', fontWeight: 'bold' }}>
          Prolific Completion Code
        </Typography>
        <Typography variant="h5" sx={{ fontWeight: 'bold', letterSpacing: 2 }}>
          C19A8F42
        </Typography>
      </Box>
    </Paper>
  );
};
