import React from 'react';
import { Box, Paper, Typography, Link } from '@mui/material';

interface CompleteScreenProps {
  completionCode?: string;
}

export const CompleteScreen: React.FC<CompleteScreenProps> = ({ completionCode }) => {
  const codeToDisplay = completionCode || 'CODE_ERROR';
  const redirectUrl = `https://app.prolific.com/submissions/complete?cc=${codeToDisplay}`;

  return (
    <Paper elevation={2} sx={{ p: 6, textAlign: 'center', borderRadius: 2 }}>
      <Typography variant="h4" color="success.main" gutterBottom>
        Study Complete
      </Typography>

      <Typography variant="body1" sx={{ color: 'text.secondary', mb: 2 }}>
        Thank you for completing this study! Your responses have been successfully recorded and will
        be invaluable to our research on human-agent interaction.
      </Typography>

      <Typography variant="body2" sx={{ color: 'text.secondary', mb: 4 }}>
        You may now safely close this window, or use the link below to automatically register your
        completion on Prolific.
      </Typography>

      <Box sx={{ bgcolor: '#e8f5e9', p: 4, borderRadius: 2, display: 'inline-block' }}>
        <Typography variant="overline" sx={{ color: 'success.dark', fontWeight: 'bold' }}>
          Prolific Completion Code
        </Typography>
        <Typography variant="h4" sx={{ fontWeight: 'bold', letterSpacing: 2, mt: 1, mb: 3 }}>
          {codeToDisplay}
        </Typography>

        <Link
          href={redirectUrl}
          target="_blank"
          rel="noopener noreferrer"
          sx={{
            fontWeight: 'bold',
            color: 'success.dark',
            textDecoration: 'none',
            borderBottom: '2px solid',
            pb: 0.5,
            '&:hover': { color: 'success.main' },
          }}
        >
          Click here to return to Prolific automatically
        </Link>
      </Box>
    </Paper>
  );
};
