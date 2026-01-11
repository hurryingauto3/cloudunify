import { useState, useEffect, useCallback } from 'react';
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
  TextField,
  InputAdornment,
  Chip,
  Menu,
  MenuItem,
  ListItemIcon,
  ListItemText,
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
import SearchIcon from '@mui/icons-material/Search';
import ClearIcon from '@mui/icons-material/Clear';
import CloudOffIcon from '@mui/icons-material/CloudOff';
import { getFiles, deleteFile, pinFile, unpinFile, searchFiles, dehydrateFile } from '../services/api';

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
  const [searchQuery, setSearchQuery] = useState('');
  const [searchResults, setSearchResults] = useState(null);
  const [isSearching, setIsSearching] = useState(false);

  // Context menu state
  const [contextMenu, setContextMenu] = useState(null);
  const [contextFile, setContextFile] = useState(null);

  useEffect(() => {
    if (!searchResults) {
      fetchFiles(currentPath);
    }
  }, [currentPath, searchResults]);

  // Debounced search effect
  useEffect(() => {
    if (!searchQuery.trim()) {
      setSearchResults(null);
      return;
    }

    const timer = setTimeout(async () => {
      setIsSearching(true);
      try {
        const response = await searchFiles(searchQuery, 50);
        setSearchResults(response.data || []);
      } catch (err) {
        console.error('Search failed:', err);
        setSearchResults([]);
      } finally {
        setIsSearching(false);
      }
    }, 300); // 300ms debounce

    return () => clearTimeout(timer);
  }, [searchQuery]);

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
    setSearchQuery('');
    setSearchResults(null);
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
    if (e) e.stopPropagation();
    if (!window.confirm(`Delete ${file.virtual_path}?`)) return;

    try {
      await deleteFile(file.virtual_path);
      setFiles((prev) => prev.filter((f) => f.id !== file.id));
      if (searchResults) {
        setSearchResults((prev) => prev.filter((f) => f.id !== file.id));
      }
    } catch (err) {
      console.error('Failed to delete file:', err);
    }
  };

  const handlePin = async (file) => {
    try {
      await pinFile(file.id);
      const updateFiles = (prev) => prev.map((f) => (f.id === file.id ? { ...f, pinned: true } : f));
      setFiles(updateFiles);
      if (searchResults) setSearchResults(updateFiles);
    } catch (err) {
      console.error('Failed to pin file:', err);
    }
  };

  const handleUnpin = async (file) => {
    try {
      await unpinFile(file.id);
      const updateFiles = (prev) => prev.map((f) => (f.id === file.id ? { ...f, pinned: false } : f));
      setFiles(updateFiles);
      if (searchResults) setSearchResults(updateFiles);
    } catch (err) {
      console.error('Failed to unpin file:', err);
    }
  };

  const handleDehydrate = async (file) => {
    try {
      const response = await dehydrateFile(file.id);
      if (response.data.status === 'dehydrated') {
        const freedBytes = formatBytes(response.data.freed_bytes || 0);
        console.log(`Dehydrated ${file.virtual_path}, freed ${freedBytes}`);
      }
    } catch (err) {
      console.error('Failed to dehydrate file:', err);
      if (err.response?.data?.code === 'file_pinned') {
        alert('Cannot dehydrate pinned file. Unpin first.');
      }
    }
  };

  // Context menu handlers
  const handleContextMenu = useCallback((event, file) => {
    event.preventDefault();
    event.stopPropagation();
    setContextMenu({ mouseX: event.clientX, mouseY: event.clientY });
    setContextFile(file);
  }, []);

  const handleCloseContextMenu = () => {
    setContextMenu(null);
    setContextFile(null);
  };

  const clearSearch = () => {
    setSearchQuery('');
    setSearchResults(null);
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
  const displayFiles = searchResults !== null ? searchResults : files;

  return (
    <Card>
      <CardContent>
        {/* Search Bar */}
        <Box sx={{ mb: 2 }}>
          <TextField
            fullWidth
            size="small"
            placeholder="Search files..."
            value={searchQuery}
            onChange={(e) => setSearchQuery(e.target.value)}
            InputProps={{
              startAdornment: (
                <InputAdornment position="start">
                  <SearchIcon color="action" />
                </InputAdornment>
              ),
              endAdornment: searchQuery && (
                <InputAdornment position="end">
                  <IconButton size="small" onClick={clearSearch}>
                    <ClearIcon fontSize="small" />
                  </IconButton>
                </InputAdornment>
              ),
            }}
          />
          {searchResults !== null && (
            <Chip
              label={`${searchResults.length} result${searchResults.length !== 1 ? 's' : ''}`}
              size="small"
              onDelete={clearSearch}
              sx={{ mt: 1 }}
            />
          )}
        </Box>

        {/* Breadcrumbs - hide during search */}
        {!searchResults && (
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
        )}

        <Stack direction="row" spacing={1} sx={{ mb: 2 }}>
          <Button
            size="small"
            variant="outlined"
            startIcon={<ArrowUpwardIcon />}
            onClick={navigateUp}
            disabled={currentPath === '/' || searchResults !== null}
          >
            Up
          </Button>
          <Button
            size="small"
            variant="outlined"
            startIcon={<RefreshIcon />}
            onClick={() => {
              if (searchResults) {
                setSearchQuery(searchQuery); // Retrigger search
              } else {
                fetchFiles(currentPath);
              }
            }}
          >
            Refresh
          </Button>
        </Stack>

        {loading || isSearching ? (
          <Typography color="text.secondary">{isSearching ? 'Searching...' : 'Loading files...'}</Typography>
        ) : error ? (
          <Typography color="error">{error}</Typography>
        ) : displayFiles.length === 0 ? (
          <Box sx={{ textAlign: 'center', py: 4 }}>
            <Typography color="text.secondary">
              {searchResults !== null ? 'No files match your search' : 'This folder is empty'}
            </Typography>
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
                {displayFiles.map((file) => (
                  <TableRow
                    key={file.id}
                    hover
                    sx={{ cursor: file.is_dir ? 'pointer' : 'default' }}
                    onClick={() => handleFileClick(file)}
                    onContextMenu={(e) => handleContextMenu(e, file)}
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
                      <Stack direction="row" spacing={0} justifyContent="flex-end">
                        {!file.is_dir && !file.pinned && (
                          <Tooltip title="Dehydrate (Remove local copy)">
                            <IconButton
                              size="small"
                              onClick={(e) => {
                                e.stopPropagation();
                                handleDehydrate(file);
                              }}
                            >
                              <CloudOffIcon fontSize="small" />
                            </IconButton>
                          </Tooltip>
                        )}
                        <Tooltip title="Delete">
                          <IconButton size="small" onClick={(e) => handleDelete(file, e)}>
                            <DeleteOutlineIcon fontSize="small" />
                          </IconButton>
                        </Tooltip>
                      </Stack>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </TableContainer>
        )}

        {/* Context Menu */}
        <Menu
          open={contextMenu !== null}
          onClose={handleCloseContextMenu}
          anchorReference="anchorPosition"
          anchorPosition={
            contextMenu !== null
              ? { top: contextMenu.mouseY, left: contextMenu.mouseX }
              : undefined
          }
        >
          {contextFile && !contextFile.pinned && (
            <MenuItem onClick={() => { handlePin(contextFile); handleCloseContextMenu(); }}>
              <ListItemIcon>
                <PushPinIcon fontSize="small" />
              </ListItemIcon>
              <ListItemText>Pin (Keep Offline)</ListItemText>
            </MenuItem>
          )}
          {contextFile && contextFile.pinned && (
            <MenuItem onClick={() => { handleUnpin(contextFile); handleCloseContextMenu(); }}>
              <ListItemIcon>
                <PushPinOutlinedIcon fontSize="small" />
              </ListItemIcon>
              <ListItemText>Unpin</ListItemText>
            </MenuItem>
          )}
          {contextFile && !contextFile.is_dir && !contextFile.pinned && (
            <MenuItem onClick={() => { handleDehydrate(contextFile); handleCloseContextMenu(); }}>
              <ListItemIcon>
                <CloudOffIcon fontSize="small" />
              </ListItemIcon>
              <ListItemText>Dehydrate (Remove local copy)</ListItemText>
            </MenuItem>
          )}
          <MenuItem onClick={() => { if (contextFile) handleDelete(contextFile); handleCloseContextMenu(); }}>
            <ListItemIcon>
              <DeleteOutlineIcon fontSize="small" />
            </ListItemIcon>
            <ListItemText>Delete</ListItemText>
          </MenuItem>
        </Menu>
      </CardContent>
    </Card>
  );
}

export default FileBrowser;
