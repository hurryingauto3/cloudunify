import { useState, useEffect } from 'react';
import {
  Box,
  Card,
  CardContent,
  Typography,
  Breadcrumbs,
  Link,
  Button,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
  IconButton,
  Tooltip,
  Stack,
} from '@mui/material';
import HomeIcon from '@mui/icons-material/Home';
import FolderIcon from '@mui/icons-material/Folder';
import InsertDriveFileIcon from '@mui/icons-material/InsertDriveFile';
import ImageIcon from '@mui/icons-material/Image';
import VideoFileIcon from '@mui/icons-material/VideoFile';
import AudioFileIcon from '@mui/icons-material/AudioFile';
import PictureAsPdfIcon from '@mui/icons-material/PictureAsPdf';
import DescriptionIcon from '@mui/icons-material/Description';
import FolderZipIcon from '@mui/icons-material/FolderZip';
import ArrowUpwardIcon from '@mui/icons-material/ArrowUpward';
import RefreshIcon from '@mui/icons-material/Refresh';
import DeleteOutlineIcon from '@mui/icons-material/DeleteOutline';
import PushPinIcon from '@mui/icons-material/PushPin';
import PushPinOutlinedIcon from '@mui/icons-material/PushPinOutlined';
import CloudIcon from '@mui/icons-material/Cloud';
import CloudDoneIcon from '@mui/icons-material/CloudDone';
import CloudUploadIcon from '@mui/icons-material/CloudUpload';
import CloudDownloadIcon from '@mui/icons-material/CloudDownload';
import ErrorOutlineIcon from '@mui/icons-material/ErrorOutline';
import { getFiles, deleteFile, pinFile, unpinFile } from '../services/api';

function formatBytes(bytes) {
  if (bytes === 0) return '0 B';
  const k = 1024;
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i];
}

function formatDate(dateString) {
  return new Date(dateString).toLocaleDateString(undefined, {
    year: 'numeric',
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  });
}

function FileBrowser() {
  const [currentPath, setCurrentPath] = useState('/');
  const [files, setFiles] = useState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);

  useEffect(() => {
    fetchFiles(currentPath);
  }, [currentPath]);

  const fetchFiles = async (path) => {
    try {
      setLoading(true);
      setError(null);
      const response = await getFiles(path);
      setFiles(response.data || []);
    } catch (err) {
      setError('Failed to load files');
      console.error(err);
    } finally {
      setLoading(false);
    }
  };

  const navigateTo = (path) => {
    setCurrentPath(path);
  };

  const navigateUp = () => {
    if (currentPath === '/') return;
    const parts = currentPath.split('/').filter(Boolean);
    parts.pop();
    setCurrentPath('/' + parts.join('/'));
  };

  const handleFileClick = (file) => {
    if (file.is_dir) {
      navigateTo(file.virtual_path);
    }
  };

  const handleDelete = async (file, e) => {
    e.stopPropagation();
    if (!window.confirm(`Delete ${file.virtual_path}?`)) return;

    try {
      await deleteFile(file.virtual_path);
      setFiles((prev) => prev.filter((f) => f.id !== file.id));
    } catch (err) {
      console.error('Failed to delete file:', err);
    }
  };

  const handlePin = async (file) => {
    try {
      await pinFile(file.id);
      setFiles((prev) => prev.map((f) => (f.id === file.id ? { ...f, pinned: true } : f)));
    } catch (err) {
      console.error('Failed to pin file:', err);
    }
  };

  const handleUnpin = async (file) => {
    try {
      await unpinFile(file.id);
      setFiles((prev) => prev.map((f) => (f.id === file.id ? { ...f, pinned: false } : f)));
    } catch (err) {
      console.error('Failed to unpin file:', err);
    }
  };

  const getStatusIcon = (status) => {
    switch (status) {
      case 'synced':
        return <CloudDoneIcon fontSize="small" color="success" />;
      case 'uploading':
        return <CloudUploadIcon fontSize="small" color="info" />;
      case 'downloading':
        return <CloudDownloadIcon fontSize="small" color="info" />;
      case 'pending':
        return <CloudIcon fontSize="small" color="disabled" />;
      case 'error':
        return <ErrorOutlineIcon fontSize="small" color="error" />;
      default:
        return <CloudIcon fontSize="small" color="disabled" />;
    }
  };

  const getFileIcon = (file) => {
    if (file.is_dir) return <FolderIcon color="primary" />;

    const ext = file.virtual_path.split('.').pop()?.toLowerCase();
    const iconMap = {
      mp4: <VideoFileIcon color="secondary" />,
      mov: <VideoFileIcon color="secondary" />,
      avi: <VideoFileIcon color="secondary" />,
      mkv: <VideoFileIcon color="secondary" />,
      mp3: <AudioFileIcon color="info" />,
      wav: <AudioFileIcon color="info" />,
      flac: <AudioFileIcon color="info" />,
      jpg: <ImageIcon color="success" />,
      jpeg: <ImageIcon color="success" />,
      png: <ImageIcon color="success" />,
      gif: <ImageIcon color="success" />,
      pdf: <PictureAsPdfIcon color="error" />,
      doc: <DescriptionIcon />,
      docx: <DescriptionIcon />,
      txt: <DescriptionIcon />,
      zip: <FolderZipIcon />,
      rar: <FolderZipIcon />,
      '7z': <FolderZipIcon />,
    };
    return iconMap[ext] || <InsertDriveFileIcon />;
  };

  const breadcrumbs = currentPath.split('/').filter(Boolean);

  return (
    <Card>
      <CardContent>
        <Box sx={{ mb: 2 }}>
          <Breadcrumbs>
            <Link
              component="button"
              variant="body2"
              underline="hover"
              onClick={() => navigateTo('/')}
              sx={{ display: 'flex', alignItems: 'center', gap: 0.5 }}
            >
              <HomeIcon fontSize="small" />
              Home
            </Link>
            {breadcrumbs.map((part, index) => (
              <Link
                key={index}
                component="button"
                variant="body2"
                underline="hover"
                onClick={() => navigateTo('/' + breadcrumbs.slice(0, index + 1).join('/'))}
              >
                {part}
              </Link>
            ))}
          </Breadcrumbs>
        </Box>

        <Stack direction="row" spacing={1} sx={{ mb: 2 }}>
          <Button
            size="small"
            variant="outlined"
            startIcon={<ArrowUpwardIcon />}
            onClick={navigateUp}
            disabled={currentPath === '/'}
          >
            Up
          </Button>
          <Button
            size="small"
            variant="outlined"
            startIcon={<RefreshIcon />}
            onClick={() => fetchFiles(currentPath)}
          >
            Refresh
          </Button>
        </Stack>

        {loading ? (
          <Typography color="text.secondary">Loading files...</Typography>
        ) : error ? (
          <Typography color="error">{error}</Typography>
        ) : files.length === 0 ? (
          <Box sx={{ textAlign: 'center', py: 4 }}>
            <Typography color="text.secondary">This folder is empty</Typography>
          </Box>
        ) : (
          <TableContainer>
            <Table size="small">
              <TableHead>
                <TableRow>
                  <TableCell>Name</TableCell>
                  <TableCell align="center">Status</TableCell>
                  <TableCell align="center">Pinned</TableCell>
                  <TableCell>Size</TableCell>
                  <TableCell>Modified</TableCell>
                  <TableCell align="right">Actions</TableCell>
                </TableRow>
              </TableHead>
              <TableBody>
                {files.map((file) => (
                  <TableRow
                    key={file.id}
                    hover
                    sx={{ cursor: file.is_dir ? 'pointer' : 'default' }}
                    onClick={() => handleFileClick(file)}
                  >
                    <TableCell>
                      <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                        {getFileIcon(file)}
                        <Typography variant="body2">
                          {file.virtual_path.split('/').pop()}
                        </Typography>
                      </Box>
                    </TableCell>
                    <TableCell align="center">
                      <Tooltip title={file.status || 'unknown'}>
                        <Box component="span" sx={{ display: 'inline-flex', verticalAlign: 'middle' }}>
                          {getStatusIcon(file.status)}
                        </Box>
                      </Tooltip>
                    </TableCell>
                    <TableCell align="center">
                      {file.pinned ? (
                        <Tooltip title="Unpin">
                          <IconButton
                            size="small"
                            onClick={(e) => {
                              e.stopPropagation();
                              handleUnpin(file);
                            }}
                            color="primary"
                          >
                            <PushPinIcon fontSize="small" />
                          </IconButton>
                        </Tooltip>
                      ) : (
                        <Tooltip title="Pin (Keep Offline)">
                          <IconButton
                            size="small"
                            onClick={(e) => {
                              e.stopPropagation();
                              handlePin(file);
                            }}
                          >
                            <PushPinOutlinedIcon fontSize="small" />
                          </IconButton>
                        </Tooltip>
                      )}
                    </TableCell>
                    <TableCell>
                      <Typography variant="body2" color="text.secondary">
                        {file.is_dir ? '-' : formatBytes(file.size_bytes)}
                      </Typography>
                    </TableCell>
                    <TableCell>
                      <Typography variant="body2" color="text.secondary">
                        {formatDate(file.updated_at)}
                      </Typography>
                    </TableCell>
                    <TableCell align="right">
                      <Tooltip title="Delete">
                        <IconButton size="small" onClick={(e) => handleDelete(file, e)}>
                          <DeleteOutlineIcon fontSize="small" />
                        </IconButton>
                      </Tooltip>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </TableContainer>
        )}
      </CardContent>
    </Card>
  );
}

export default FileBrowser;
