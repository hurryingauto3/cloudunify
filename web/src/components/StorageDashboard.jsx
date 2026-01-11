import { useState, useEffect } from 'react';
import {
    Box,
    Card,
    CardContent,
    Typography,
    LinearProgress,
    Grid,
} from '@mui/material';
import { getStorageStats } from '../services/api';
import ProviderCard from './ProviderCard';

function formatBytes(bytes) {
    if (bytes === 0) return '0 B';
    const k = 1024;
    const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
    const i = Math.floor(Math.log(bytes) / Math.log(k));
    return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i];
}

function StorageDashboard({ providers, onRemoveProvider, onRefreshProvider }) {
    const [stats, setStats] = useState(null);
    const [loading, setLoading] = useState(true);
    const [error, setError] = useState(null);

    // Check if iCloud is among providers (has estimated quota)
    const hasICloud = providers.some(p => p.type === 'icloud');

    useEffect(() => {
        fetchStats();
    }, [providers]);

    const fetchStats = async () => {
        try {
            setLoading(true);
            const response = await getStorageStats();
            setStats(response.data);
            setError(null);
        } catch (err) {
            setError('Failed to load storage statistics');
            console.error(err);
        } finally {
            setLoading(false);
        }
    };

    if (loading && !stats) {
        return (
            <Card>
                <CardContent>
                    <Typography color="text.secondary">Loading storage statistics...</Typography>
                </CardContent>
            </Card>
        );
    }

    if (error) {
        return (
            <Card>
                <CardContent>
                    <Typography color="error">{error}</Typography>
                </CardContent>
            </Card>
        );
    }

    const usagePercent = stats ? (stats.used_bytes / stats.total_bytes) * 100 : 0;

    return (
        <Box>
            <Card sx={{ mb: 3 }}>
                <CardContent>
                    <Typography variant="h2" gutterBottom>
                        Total Storage
                    </Typography>
                    <LinearProgress
                        variant="determinate"
                        value={Math.min(usagePercent, 100)}
                        sx={{ height: 12, borderRadius: 1, mb: 2 }}
                    />
                    <Box sx={{ display: 'flex', justifyContent: 'space-between', flexWrap: 'wrap', gap: 2 }}>
                        <Typography variant="body2" color="text.secondary">
                            <strong>{formatBytes(stats?.used_bytes || 0)}</strong> used
                        </Typography>
                        <Typography variant="body2" color="text.secondary">
                            of <strong>{formatBytes(stats?.total_bytes || 0)}</strong>
                        </Typography>
                        <Typography variant="body2" color="success.main">
                            <strong>{formatBytes(stats?.free_bytes || 0)}</strong> available
                        </Typography>
                    </Box>
                    {hasICloud && (
                        <Typography variant="caption" color="text.secondary" sx={{ mt: 1, display: 'block' }}>
                            * iCloud quota is estimated. Actual quota managed by Apple.
                        </Typography>
                    )}
                </CardContent>
            </Card>

            <Typography variant="h2" gutterBottom>
                Connected Providers
            </Typography>

            {providers.length === 0 ? (
                <Card>
                    <CardContent sx={{ textAlign: 'center', py: 4 }}>
                        <Typography color="text.secondary">
                            No storage providers connected.
                        </Typography>
                        <Typography variant="body2" color="text.secondary">
                            Add a provider to get started!
                        </Typography>
                    </CardContent>
                </Card>
            ) : (
                <Grid container spacing={2}>
                    {providers.map((provider) => (
                        <Grid item xs={12} md={6} lg={4} key={provider.id}>
                            <ProviderCard
                                provider={provider}
                                onRefresh={onRefreshProvider}
                            />
                        </Grid>
                    ))}
                </Grid>
            )}
        </Box>
    );
}

export default StorageDashboard;
