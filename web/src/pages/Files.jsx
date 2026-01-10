import { Box, Typography } from '@mui/material';
import FileBrowser from '../components/FileBrowser';

function Files() {
  return (
    <Box>
      <Typography variant="h1" sx={{ mb: 3 }}>
        Files
      </Typography>
      <FileBrowser />
    </Box>
  );
}

export default Files;
