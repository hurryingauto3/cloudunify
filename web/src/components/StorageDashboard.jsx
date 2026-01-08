import { useState, useEffect } from 'react';
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
        return <div className="loading">Loading storage statistics...</div>;
    }

    if (error) {
        return <div className="error">{error}</div>;
    }

    const usagePercent = stats ? (stats.used_bytes / stats.total_bytes) * 100 : 0;

    return (
        <div className="storage-dashboard">
            <div className="total-storage">
                <h2>Total Storage</h2>
                <div className="storage-overview">
                    <div className="storage-bar large">
                        <div
                            className="storage-used"
                            style={{ width: `${Math.min(usagePercent, 100)}%` }}
                        />
                    </div>
                    <div className="storage-numbers">
                        <span className="used">{formatBytes(stats?.used_bytes || 0)} used</span>
                        <span className="total">of {formatBytes(stats?.total_bytes || 0)}</span>
                        <span className="free">{formatBytes(stats?.free_bytes || 0)} available</span>
                    </div>
                </div>
            </div>

            <div className="providers-section">
                <h2>Connected Providers</h2>
                {providers.length === 0 ? (
                    <div className="empty-state">
                        <p>No storage providers connected.</p>
                        <p>Add a provider to get started!</p>
                    </div>
                ) : (
                    <div className="providers-grid">
                        {providers.map((provider) => (
                            <ProviderCard
                                key={provider.id}
                                provider={provider}
                                onRemove={onRemoveProvider}
                                onRefresh={onRefreshProvider}
                            />
                        ))}
                    </div>
                )}
            </div>
        </div>
    );
}

export default StorageDashboard;
