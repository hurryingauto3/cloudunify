import { useState } from 'react';
import {
    Card,
    CardContent,
    Box,
    Typography,
    LinearProgress,
    Button,
    Chip,
    Stack,
} from '@mui/material';
import GoogleIcon from '@mui/icons-material/Google';
import CloudIcon from '@mui/icons-material/Cloud';
import AppleIcon from '@mui/icons-material/Apple';
import RefreshIcon from '@mui/icons-material/Refresh';
import DeleteOutlineIcon from '@mui/icons-material/DeleteOutline';

const providerIcons = {
    google_drive: <GoogleIcon />,
    onedrive: <CloudIcon />,
    icloud: <AppleIcon />,
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

    // iCloud uses local folder, quota is estimated/managed by Apple
    const isICloud = provider.type === 'icloud';
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
        <Card sx={{ height: '100%', display: 'flex', flexDirection: 'column' }}>
            <CardContent sx={{ flex: 1, display: 'flex', flexDirection: 'column' }}>
                <Box sx={{ display: 'flex', alignItems: 'flex-start', gap: 2, mb: 2 }}>
                    <Box
                        sx={{
                            width: 40,
                            height: 40,
                            borderRadius: 1,
                            bgcolor: 'primary.50',
                            display: 'flex',
                            alignItems: 'center',
                            justifyContent: 'center',
                            color: 'primary.main',
                        }}
                    >
                        {providerIcons[provider.type] || <CloudIcon />}
                    </Box>
                    <Box sx={{ flex: 1, minHeight: 48 }}>
                        <Typography variant="subtitle1" fontWeight={600}>
                            {providerNames[provider.type] || provider.type}
                        </Typography>
                        {isICloud ? (
                            <Typography variant="caption" color="text.secondary" display="block">
                                Local folder (synced by {navigator.platform?.includes('Mac') ? 'macOS' : 'iCloud for Windows'})
                            </Typography>
                        ) : (
                            <Typography variant="caption" color="text.secondary" display="block">
                                &nbsp;
                            </Typography>
                        )}
                    </Box>
                    <Chip
                        label={provider.enabled ? 'Connected' : 'Disconnected'}
                        size="small"
                        color={provider.enabled ? 'success' : 'error'}
                        variant="outlined"
                    />
                </Box>

                <Box sx={{ mb: 2, flex: 1 }}>
                    {isICloud ? (
                        // iCloud shows folder usage, not true quota
                        <>
                            <Box sx={{
                                p: 1.5,
                                bgcolor: 'grey.100',
                                borderRadius: 1,
                                display: 'flex',
                                justifyContent: 'space-between',
                                alignItems: 'center',
                                mb: 1
                            }}>
                                <Typography variant="body2" color="text.secondary">
                                    Folder usage
                                </Typography>
                                <Typography variant="body2" fontWeight={500}>
                                    {formatBytes(provider.used_bytes)}
                                </Typography>
                            </Box>
                            <Typography variant="caption" color="text.secondary" sx={{ display: 'block' }}>
                                Quota managed by iCloud. Check Apple Settings for storage details.
                            </Typography>
                        </>
                    ) : (
                        // Google Drive / OneDrive show real quota
                        <>
                            <LinearProgress
                                variant="determinate"
                                value={Math.min(usagePercent, 100)}
                                sx={{ height: 8, borderRadius: 1, mb: 1 }}
                            />
                            <Box sx={{ display: 'flex', justifyContent: 'space-between' }}>
                                <Typography variant="caption" color="text.secondary">
                                    {formatBytes(provider.used_bytes)} used
                                </Typography>
                                <Typography variant="caption" color="text.secondary">
                                    {formatBytes(provider.quota_bytes - provider.used_bytes)} free
                                </Typography>
                            </Box>
                        </>
                    )}
                </Box>

                <Stack direction="row" spacing={1} sx={{ mt: 'auto' }}>
                    <Button
                        size="small"
                        variant="outlined"
                        startIcon={<RefreshIcon />}
                        onClick={() => onRefresh(provider.id)}
                    >
                        Refresh
                    </Button>
                    {onRemove && (
                        <Button
                            size="small"
                            variant="outlined"
                            color="error"
                            startIcon={<DeleteOutlineIcon />}
                            onClick={handleRemove}
                            disabled={isRemoving}
                        >
                            {isRemoving ? 'Removing...' : 'Remove'}
                        </Button>
                    )}
                </Stack>
            </CardContent>
        </Card>
    );
}

export default ProviderCard;
