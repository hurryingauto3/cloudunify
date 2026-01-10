import { useState, useEffect } from 'react';
import {
  Box,
  Card,
  CardContent,
  Typography,
  Button,
  Stepper,
  Step,
  StepLabel,
  Chip,
  Alert,
  Stack,
  CircularProgress,
} from '@mui/material';
import GoogleIcon from '@mui/icons-material/Google';
import CloudIcon from '@mui/icons-material/Cloud';
import AppleIcon from '@mui/icons-material/Apple';
import CheckCircleIcon from '@mui/icons-material/CheckCircle';
import { addProvider, getOAuthStatus, getProviders } from '../services/api';

const providerOptions = [
  {
    type: 'google_drive',
    name: 'Google Drive',
    icon: <GoogleIcon />,
    description: 'Connect your Google Drive account',
    quota: '15 GB free',
  },
  {
    type: 'onedrive',
    name: 'OneDrive',
    icon: <CloudIcon />,
    description: 'Connect your Microsoft OneDrive account',
    quota: '5 GB free',
  },
  {
    type: 'icloud',
    name: 'iCloud',
    icon: <AppleIcon />,
    description: 'Connect your Apple iCloud account',
    quota: '5 GB free',
  },
];

const steps = ['Select Providers', 'Connect', 'Ready'];

function SetupWizard({ onComplete }) {
  const [activeStep, setActiveStep] = useState(0);
  const [selectedProviders, setSelectedProviders] = useState([]);
  const [connecting, setConnecting] = useState(false);
  const [connectedProviders, setConnectedProviders] = useState([]);
  const [error, setError] = useState(null);
  const [oauthStatus, setOauthStatus] = useState({});
  const [pendingProvider, setPendingProvider] = useState(null);

  const checkOAuthStatus = async () => {
    try {
      const response = await getOAuthStatus();
      setOauthStatus(response.data);
    } catch (err) {
      console.error('Failed to get OAuth status:', err);
    }
  };

  const loadExistingProviders = async () => {
    try {
      const response = await getProviders();
      const providers = response.data;
      const connected = providers.filter((p) => p.enabled || p.is_authenticated).map((p) => p.type);
      setConnectedProviders(connected);
    } catch (err) {
      console.error('Failed to load existing providers:', err);
    }
  };

  useEffect(() => {
    const params = new URLSearchParams(window.location.search);
    const success = params.get('success');
    const errorParam = params.get('error');

    if (success === 'true') {
      loadExistingProviders();
      window.history.replaceState({}, '', window.location.pathname);
    } else if (errorParam) {
      const errorDesc = params.get('error_description') || params.get('message') || errorParam;
      setError(`OAuth error: ${errorDesc}`);
      window.history.replaceState({}, '', window.location.pathname);
    }

    checkOAuthStatus();
    loadExistingProviders();
  }, []);

  useEffect(() => {
    if (pendingProvider) {
      const interval = setInterval(async () => {
        try {
          const response = await getProviders();
          const providers = response.data;
          const provider = providers.find((p) => p.id === pendingProvider);
          if (provider && provider.enabled) {
            setConnectedProviders((prev) => [...new Set([...prev, provider.type])]);
            setPendingProvider(null);
            setConnecting(false);
          }
        } catch (err) {
          console.error('Failed to check provider status:', err);
        }
      }, 2000);
      return () => clearInterval(interval);
    }
  }, [pendingProvider]);

  const toggleProvider = (type) => {
    setSelectedProviders((prev) =>
      prev.includes(type) ? prev.filter((p) => p !== type) : [...prev, type]
    );
  };

  const connectProvider = async (type) => {
    setConnecting(true);
    setError(null);

    try {
      if (!oauthStatus[type]?.configured) {
        setError(`OAuth not configured for ${type}. Please set environment variables.`);
        setConnecting(false);
        return;
      }

      const response = await addProvider(type);
      const { provider, auth_url, message } = response.data;

      if (auth_url) {
        setPendingProvider(provider.id);

        const width = 600;
        const height = 700;
        const left = window.screenX + (window.outerWidth - width) / 2;
        const top = window.screenY + (window.outerHeight - height) / 2;

        const popup = window.open(
          auth_url,
          'oauth_popup',
          `width=${width},height=${height},left=${left},top=${top},menubar=no,toolbar=no,location=no,status=no`
        );

        if (!popup || popup.closed) {
          setError('Popup was blocked. Please allow popups for this site and try again.');
          setConnecting(false);
          setPendingProvider(null);
          return;
        }

        const checkPopup = setInterval(() => {
          if (popup.closed) {
            clearInterval(checkPopup);
          }
        }, 500);
      } else if (message) {
        setError(message);
        setConnecting(false);
      } else {
        setConnectedProviders((prev) => [...prev, type]);
        setConnecting(false);
      }
    } catch (err) {
      const errorMessage = err.response?.data?.error?.message || err.message;
      setError(`Failed to connect ${type}: ${errorMessage}`);
      setConnecting(false);
      setPendingProvider(null);
    }
  };

  const handleNext = () => {
    if (activeStep === 0 && selectedProviders.length > 0) {
      setActiveStep(1);
    } else if (activeStep === 1) {
      setActiveStep(2);
    }
  };

  const handleBack = () => {
    setActiveStep((prev) => prev - 1);
  };

  const isProviderConfigured = (type) => {
    return oauthStatus[type]?.configured !== false;
  };

  return (
    <Box sx={{ display: 'flex', justifyContent: 'center', alignItems: 'center', minHeight: '100vh', bgcolor: 'background.default', p: 3 }}>
      <Card sx={{ maxWidth: 600, width: '100%' }}>
        <CardContent sx={{ p: 4 }}>
          <Box sx={{ textAlign: 'center', mb: 4 }}>
            <CloudIcon sx={{ fontSize: 48, color: 'primary.main', mb: 2 }} />
            <Typography variant="h4" fontWeight={600} gutterBottom>
              Welcome to CloudUnify
            </Typography>
            <Typography color="text.secondary">
              Unify your cloud storage into one virtual drive
            </Typography>
          </Box>

          <Stepper activeStep={activeStep} sx={{ mb: 4 }}>
            {steps.map((label) => (
              <Step key={label}>
                <StepLabel>{label}</StepLabel>
              </Step>
            ))}
          </Stepper>

          {activeStep === 0 && (
            <Box>
              <Typography variant="h6" gutterBottom>
                Select your cloud storage providers
              </Typography>
              <Typography variant="body2" color="text.secondary" sx={{ mb: 3 }}>
                Choose which services you want to unify
              </Typography>

              <Stack spacing={2} sx={{ mb: 3 }}>
                {providerOptions.map((provider) => {
                  const isConfigured = isProviderConfigured(provider.type);
                  const isConnected = connectedProviders.includes(provider.type);
                  const isSelected = selectedProviders.includes(provider.type);

                  return (
                    <Box
                      key={provider.type}
                      onClick={() => !isConnected && isConfigured && toggleProvider(provider.type)}
                      sx={{
                        p: 2,
                        border: 2,
                        borderColor: isConnected ? 'success.main' : isSelected ? 'primary.main' : 'grey.200',
                        borderRadius: 2,
                        cursor: isConnected || !isConfigured ? 'default' : 'pointer',
                        opacity: !isConfigured ? 0.5 : 1,
                        display: 'flex',
                        alignItems: 'center',
                        gap: 2,
                        transition: 'all 0.2s',
                        '&:hover': isConfigured && !isConnected ? { borderColor: 'primary.main' } : {},
                      }}
                    >
                      <Box sx={{ color: 'primary.main' }}>{provider.icon}</Box>
                      <Box sx={{ flex: 1 }}>
                        <Typography fontWeight={500}>{provider.name}</Typography>
                        <Typography variant="body2" color="text.secondary">
                          {provider.description}
                        </Typography>
                        <Chip label={provider.quota} size="small" sx={{ mt: 1 }} />
                        {!isConfigured && (
                          <Chip label="OAuth not configured" size="small" color="warning" sx={{ mt: 1, ml: 1 }} />
                        )}
                        {isConnected && (
                          <Chip label="Already connected" size="small" color="success" sx={{ mt: 1, ml: 1 }} />
                        )}
                      </Box>
                      {(isConnected || isSelected) && (
                        <CheckCircleIcon color={isConnected ? 'success' : 'primary'} />
                      )}
                    </Box>
                  );
                })}
              </Stack>

              <Button
                variant="contained"
                fullWidth
                onClick={handleNext}
                disabled={selectedProviders.length === 0 && connectedProviders.length === 0}
              >
                {connectedProviders.length > 0 && selectedProviders.length === 0 ? 'Continue with existing' : 'Continue'}
              </Button>
            </Box>
          )}

          {activeStep === 1 && (
            <Box>
              <Typography variant="h6" gutterBottom>
                Connect your accounts
              </Typography>
              <Typography variant="body2" color="text.secondary" sx={{ mb: 3 }}>
                Sign in to each service to grant CloudUnify access
              </Typography>

              {error && (
                <Alert severity="error" sx={{ mb: 2 }}>
                  {error}
                </Alert>
              )}

              <Stack spacing={2} sx={{ mb: 3 }}>
                {selectedProviders.map((type) => {
                  const provider = providerOptions.find((p) => p.type === type);
                  const isConnected = connectedProviders.includes(type);
                  const isPending = pendingProvider && connecting;

                  return (
                    <Box
                      key={type}
                      sx={{
                        p: 2,
                        border: 1,
                        borderColor: 'grey.200',
                        borderRadius: 2,
                        display: 'flex',
                        alignItems: 'center',
                        gap: 2,
                      }}
                    >
                      <Box sx={{ color: 'primary.main' }}>{provider.icon}</Box>
                      <Typography sx={{ flex: 1 }}>{provider.name}</Typography>
                      {isConnected ? (
                        <Chip label="Connected" color="success" icon={<CheckCircleIcon />} />
                      ) : (
                        <Button
                          variant="outlined"
                          onClick={() => connectProvider(type)}
                          disabled={connecting}
                          startIcon={isPending ? <CircularProgress size={16} /> : null}
                        >
                          {isPending ? 'Waiting...' : connecting ? 'Connecting...' : 'Connect'}
                        </Button>
                      )}
                    </Box>
                  );
                })}
              </Stack>

              <Stack direction="row" spacing={2}>
                <Button variant="outlined" onClick={handleBack}>
                  Back
                </Button>
                <Button
                  variant="contained"
                  fullWidth
                  onClick={handleNext}
                  disabled={connectedProviders.length === 0 && selectedProviders.some((t) => !connectedProviders.includes(t))}
                >
                  {connectedProviders.length > 0 ? 'Continue' : 'Skip for now'}
                </Button>
              </Stack>
            </Box>
          )}

          {activeStep === 2 && (
            <Box sx={{ textAlign: 'center' }}>
              <CheckCircleIcon sx={{ fontSize: 64, color: 'success.main', mb: 2 }} />
              <Typography variant="h5" fontWeight={600} gutterBottom>
                You're all set!
              </Typography>
              <Typography color="text.secondary" sx={{ mb: 3 }}>
                CloudUnify is ready to use. Your unified storage is mounted at{' '}
                <code>~/CloudUnify</code>
              </Typography>

              {connectedProviders.length > 0 && (
                <Box sx={{ mb: 3 }}>
                  <Typography variant="subtitle2" gutterBottom>
                    Connected Providers
                  </Typography>
                  <Stack direction="row" spacing={1} justifyContent="center">
                    {connectedProviders.map((type) => {
                      const provider = providerOptions.find((p) => p.type === type);
                      return provider ? (
                        <Chip key={type} icon={provider.icon} label={provider.name} />
                      ) : null;
                    })}
                  </Stack>
                </Box>
              )}

              {connectedProviders.length === 0 && (
                <Alert severity="info" sx={{ mb: 3 }}>
                  No providers connected yet. You can add them later from Settings.
                </Alert>
              )}

              <Button variant="contained" size="large" onClick={onComplete}>
                Go to Dashboard
              </Button>
            </Box>
          )}
        </CardContent>
      </Card>
    </Box>
  );
}

export default SetupWizard;
