import { useState, useEffect } from 'react';
import { getSyncQueue, cancelSyncItem } from '../services/api';
import wsService from '../services/websocket';

function SyncProgress() {
  const [queue, setQueue] = useState([]);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    fetchQueue();

    // Subscribe to real-time sync updates
    const unsubProgress = wsService.on('sync_progress', (data) => {
      setQueue((prev) =>
        prev.map((item) =>
          item.id === data.id ? { ...item, progress_percent: data.progress } : item
        )
      );
    });

    const unsubComplete = wsService.on('sync_complete', (data) => {
      setQueue((prev) => prev.filter((item) => item.id !== data.id));
    });

    return () => {
      unsubProgress();
      unsubComplete();
    };
  }, []);

  const fetchQueue = async () => {
    try {
      const response = await getSyncQueue();
      setQueue(response.data || []);
    } catch (err) {
      console.error('Failed to fetch sync queue:', err);
    } finally {
      setLoading(false);
    }
  };

  const handleCancel = async (id) => {
    try {
      await cancelSyncItem(id);
      setQueue((prev) => prev.filter((item) => item.id !== id));
    } catch (err) {
      console.error('Failed to cancel sync item:', err);
    }
  };

  const getOperationIcon = (operation) => {
    switch (operation) {
      case 'upload':
        return '⬆️';
      case 'download':
        return '⬇️';
      case 'delete':
        return '🗑️';
      default:
        return '🔄';
    }
  };

  const getStatusColor = (status) => {
    switch (status) {
      case 'processing':
        return 'blue';
      case 'pending':
        return 'gray';
      case 'failed':
        return 'red';
      case 'completed':
        return 'green';
      default:
        return 'gray';
    }
  };

  if (loading) {
    return <div className="loading">Loading sync queue...</div>;
  }

  if (queue.length === 0) {
    return (
      <div className="sync-progress empty">
        <p>No active sync operations</p>
      </div>
    );
  }

  return (
    <div className="sync-progress">
      <h3>Sync Queue ({queue.length})</h3>
      <div className="sync-list">
        {queue.map((item) => (
          <div key={item.id} className={`sync-item status-${getStatusColor(item.status)}`}>
            <div className="sync-item-header">
              <span className="operation-icon">{getOperationIcon(item.operation)}</span>
              <span className="file-path">{item.virtual_path}</span>
              <span className={`status-badge ${item.status}`}>{item.status}</span>
            </div>

            {item.status === 'processing' && (
              <div className="progress-bar">
                <div
                  className="progress-fill"
                  style={{ width: `${item.progress_percent}%` }}
                />
                <span className="progress-text">{item.progress_percent}%</span>
              </div>
            )}

            {item.error_message && (
              <div className="error-message">{item.error_message}</div>
            )}

            {(item.status === 'pending' || item.status === 'failed') && (
              <button
                className="btn btn-small btn-danger"
                onClick={() => handleCancel(item.id)}
              >
                Cancel
              </button>
            )}
          </div>
        ))}
      </div>
    </div>
  );
}

export default SyncProgress;
