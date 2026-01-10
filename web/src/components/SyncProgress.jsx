import { useState, useEffect } from 'react';
import {
  Box,
  Card,
  CardContent,
  Typography,
  LinearProgress,
  Chip,
  IconButton,
  Stack,
  Tooltip,
} from '@mui/material';
import CloudUploadIcon from '@mui/icons-material/CloudUpload';
import CloudDownloadIcon from '@mui/icons-material/CloudDownload';
import DeleteIcon from '@mui/icons-material/Delete';
import SyncIcon from '@mui/icons-material/Sync';
import CloseIcon from '@mui/icons-material/Close';
import CheckCircleIcon from '@mui/icons-material/CheckCircle';
import WarningIcon from '@mui/icons-material/Warning';
import { getSyncQueue, cancelSyncItem } from '../services/api';
import wsService from '../services/websocket';

function SyncProgress() {
  const [queue, setQueue] = useState([]);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    fetchQueue();

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
        return <CloudUploadIcon fontSize="small" color="primary" />;
      case 'download':
        return <CloudDownloadIcon fontSize="small" color="info" />;
      case 'delete':
        return <DeleteIcon fontSize="small" color="error" />;
      default:
        return <SyncIcon fontSize="small" />;
    }
  };

  const getStatusChip = (status) => {
    const props = {
      processing: { label: 'Processing', color: 'primary', size: 'small' },
      pending: { label: 'Pending', color: 'default', size: 'small' },
      failed: { label: 'Failed', color: 'error', size: 'small' },
      completed: { label: 'Completed', color: 'success', size: 'small' },
    };
    return <Chip {...(props[status] || props.pending)} />;
  };

  const formatSize = (bytes) => {
    if (!bytes) return '';
    if (bytes < 1024) return `${bytes} B`;
    if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
    return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
  };

  if (loading) {
    return (
      <Card>
        <CardContent>
          <Typography color="text.secondary">Loading sync queue...</Typography>
        </CardContent>
      </Card>
    );
  }

  if (queue.length === 0) {
    return (
      <Card>
        <CardContent sx={{ textAlign: 'center', py: 4 }}>
          <CheckCircleIcon sx={{ fontSize: 40, color: 'success.main', mb: 1 }} />
          <Typography color="text.secondary">All synced</Typography>
        </CardContent>
      </Card>
    );
  }

  return (
    <Card>
      <CardContent>
        <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, mb: 2 }}>
          <Typography variant="h3">Sync Queue</Typography>
          <Chip label={queue.length} size="small" />
        </Box>

        <Stack
          spacing={1}
          sx={{
            maxHeight: 300,
            overflowY: 'auto',
            '&::-webkit-scrollbar': { width: 6 },
            '&::-webkit-scrollbar-thumb': { backgroundColor: 'grey.300', borderRadius: 3 },
          }}
        >
          {queue.map((item) => (
            <Box
              key={item.id}
              sx={{
                display: 'flex',
                alignItems: 'center',
                gap: 1.5,
                p: 1.5,
                borderRadius: 1,
                bgcolor: item.status === 'failed' ? 'error.50' : item.status === 'processing' ? 'primary.50' : 'grey.50',
                border: 1,
                borderColor: item.status === 'failed' ? 'error.200' : item.status === 'processing' ? 'primary.200' : 'grey.200',
              }}
            >
              {getOperationIcon(item.operation)}

              <Box sx={{ flex: 1, minWidth: 0 }}>
                <Typography
                  variant="body2"
                  fontWeight={500}
                  noWrap
                  title={item.virtual_path}
                >
                  {item.virtual_path}
                </Typography>

                {item.status === 'processing' && (
                  <LinearProgress
                    variant="determinate"
                    value={item.progress_percent || 0}
                    sx={{ mt: 0.5, height: 4, borderRadius: 1 }}
                  />
                )}

                {item.error_message && (
                  <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5, mt: 0.5 }}>
                    <WarningIcon sx={{ fontSize: 14, color: 'error.main' }} />
                    <Typography variant="caption" color="error.main">
                      {item.error_message}
                    </Typography>
                  </Box>
                )}

                <Box sx={{ display: 'flex', gap: 1, mt: 0.5 }}>
                  {item.file_size > 0 && (
                    <Typography variant="caption" color="text.secondary">
                      {formatSize(item.file_size)}
                    </Typography>
                  )}
                  {item.status === 'processing' && item.progress_percent > 0 && (
                    <Typography variant="caption" color="text.secondary">
                      {item.progress_percent}%
                    </Typography>
                  )}
                  {item.retry_count > 0 && (
                    <Typography variant="caption" color="text.secondary">
                      Retry {item.retry_count}
                    </Typography>
                  )}
                </Box>
              </Box>

              {getStatusChip(item.status)}

              {(item.status === 'pending' || item.status === 'failed') && (
                <Tooltip title="Cancel">
                  <IconButton size="small" onClick={() => handleCancel(item.id)}>
                    <CloseIcon fontSize="small" />
                  </IconButton>
                </Tooltip>
              )}
            </Box>
          ))}
        </Stack>
      </CardContent>
    </Card>
  );
}

export default SyncProgress;
