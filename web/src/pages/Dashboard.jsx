import { useState, useEffect } from 'react';
import { getProviders, deleteProvider, refreshProviderToken } from '../services/api';
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

  const handleRemoveProvider = async (id) => {
    try {
      await deleteProvider(id);
      setProviders((prev) => prev.filter((p) => p.id !== id));
    } catch (err) {
      console.error('Failed to remove provider:', err);
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
    return <div className="loading">Loading...</div>;
  }

  return (
    <div className="dashboard-page">
      <StorageDashboard
        providers={providers}
        onRemoveProvider={handleRemoveProvider}
        onRefreshProvider={handleRefreshProvider}
      />
      <SyncProgress />
    </div>
  );
}

export default Dashboard;
