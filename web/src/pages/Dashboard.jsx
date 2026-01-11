import { useState, useEffect } from 'react';
import { Box, Typography, Grid, CircularProgress } from '@mui/material';
import { getProviders, refreshProviderToken } from '../services/api';
import StorageDashboard from '../components/StorageDashboard';
import SyncProgress from '../components/SyncProgress';
import wsService from '../services/websocket';

function Dashboard() {
  const [providers, setProviders] = useState([]);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    fetchProviders();
    wsService.connect();

    const unsubProviderUpdate = wsService.on('provider_updated', () => {
      fetchProviders();
    });

    return () => {
      unsubProviderUpdate();
    };
  }, []);

  const fetchProviders = async () => {
    try {
      const response = await getProviders();
      setProviders(response.data || []);
    } catch (err) {
      console.error('Failed to fetch providers:', err);
    } finally {
      setLoading(false);
    }
  };

  const handleRefreshProvider = async (id) => {
    try {
      await refreshProviderToken(id);
      await fetchProviders();
    } catch (err) {
      console.error('Failed to refresh provider:', err);
    }
  };

  if (loading) {
    return (
      <Box sx={{ display: 'flex', justifyContent: 'center', alignItems: 'center', height: '50vh' }}>
        <CircularProgress />
      </Box>
    );
  }

  return (
    <Box>
      <Typography variant="h1" sx={{ mb: 3 }}>
        Dashboard
      </Typography>
      <Grid container spacing={3}>
        <Grid item xs={12} lg={8}>
          <StorageDashboard
            providers={providers}
            onRefreshProvider={handleRefreshProvider}
          />
        </Grid>
        <Grid item xs={12} lg={4}>
          <SyncProgress />
        </Grid>
      </Grid>
    </Box>
  );
}

export default Dashboard;
