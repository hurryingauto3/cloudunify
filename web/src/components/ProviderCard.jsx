import { useState } from 'react';

const providerIcons = {
    google_drive: '📁',
    onedrive: '☁️',
    icloud: '🍎',
};

const providerNames = {
    google_drive: 'Google Drive',
    onedrive: 'OneDrive',
    icloud: 'iCloud',
};

function formatBytes(bytes) {
    if (bytes === 0) return '0 B';
    const k = 1024;
    const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
    const i = Math.floor(Math.log(bytes) / Math.log(k));
    return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i];
}

function ProviderCard({ provider, onRemove, onRefresh }) {
    const [isRemoving, setIsRemoving] = useState(false);

    const usagePercent = provider.quota_bytes > 0
        ? (provider.used_bytes / provider.quota_bytes) * 100
        : 0;

    const handleRemove = async () => {
        if (window.confirm(`Remove ${provider.name}? Files will remain in the cloud.`)) {
            setIsRemoving(true);
            try {
                await onRemove(provider.id);
            } finally {
                setIsRemoving(false);
            }
        }
    };

    return (
        <div className="provider-card">
            <div className="provider-header">
                <span className="provider-icon">{providerIcons[provider.type] || '📦'}</span>
                <div className="provider-info">
                    <h3>{provider.name}</h3>
                    <span className="provider-type">{providerNames[provider.type] || provider.type}</span>
                </div>
                <span className={`status-badge ${provider.enabled ? 'connected' : 'disconnected'}`}>
                    {provider.enabled ? 'Connected' : 'Disconnected'}
                </span>
            </div>

            <div className="storage-bar">
                <div
                    className="storage-used"
                    style={{ width: `${Math.min(usagePercent, 100)}%` }}
                />
            </div>

            <div className="storage-info">
                <span>{formatBytes(provider.used_bytes)} used</span>
                <span>{formatBytes(provider.quota_bytes - provider.used_bytes)} free</span>
            </div>

            <div className="provider-actions">
                <button onClick={() => onRefresh(provider.id)} className="btn btn-secondary">
                    Refresh
                </button>
                <button
                    onClick={handleRemove}
                    className="btn btn-danger"
                    disabled={isRemoving}
                >
                    {isRemoving ? 'Removing...' : 'Remove'}
                </button>
            </div>
        </div>
    );
}

export default ProviderCard;
