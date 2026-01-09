import { useState, useEffect } from 'react';
import { addProvider, getOAuthStatus, getProviders } from '../services/api';

const providerOptions = [
  {
    type: 'google_drive',
    name: 'Google Drive',
    icon: '📁',
    description: 'Connect your Google Drive account',
    quota: '15 GB free',
  },
  {
    type: 'onedrive',
    name: 'OneDrive',
    icon: '☁️',
    description: 'Connect your Microsoft OneDrive account',
    quota: '5 GB free',
  },
  {
    type: 'icloud',
    name: 'iCloud',
    icon: '🍎',
    description: 'Connect your Apple iCloud account',
    quota: '5 GB free',
  },
];

function SetupWizard({ onComplete }) {
  const [step, setStep] = useState(1);
  const [selectedProviders, setSelectedProviders] = useState([]);
  const [connecting, setConnecting] = useState(false);
  const [connectedProviders, setConnectedProviders] = useState([]);
  const [error, setError] = useState(null);
  const [oauthStatus, setOauthStatus] = useState({});
  const [pendingProvider, setPendingProvider] = useState(null);

  // Check OAuth status on mount
  useEffect(() => {
    checkOAuthStatus();
    checkUrlParams();
    loadExistingProviders();
  }, []);

  // Poll for OAuth completion when we have a pending provider
  useEffect(() => {
    if (pendingProvider) {
      const interval = setInterval(async () => {
        try {
          const response = await getProviders();
          const providers = response.data;
          const provider = providers.find(p => p.id === pendingProvider);
          if (provider && provider.enabled) {
            setConnectedProviders(prev => [...new Set([...prev, provider.type])]);
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
      const connected = providers
        .filter(p => p.enabled || p.is_authenticated)
        .map(p => p.type);
      setConnectedProviders(connected);
    } catch (err) {
      console.error('Failed to load existing providers:', err);
    }
  };

  const checkUrlParams = () => {
    const params = new URLSearchParams(window.location.search);
    const success = params.get('success');
    const errorParam = params.get('error');

    if (success === 'true') {
      // OAuth was successful, reload providers
      loadExistingProviders();
      // Clear URL params
      window.history.replaceState({}, '', window.location.pathname);
    } else if (errorParam) {
      const errorDesc = params.get('error_description') || params.get('message') || errorParam;
      setError(`OAuth error: ${errorDesc}`);
      // Clear URL params
      window.history.replaceState({}, '', window.location.pathname);
    }
  };

  const toggleProvider = (type) => {
    setSelectedProviders((prev) =>
      prev.includes(type)
        ? prev.filter((p) => p !== type)
        : [...prev, type]
    );
  };

  const connectProvider = async (type) => {
    setConnecting(true);
    setError(null);

    try {
      // Check if OAuth is configured for this provider
      if (!oauthStatus[type]?.configured) {
        setError(`OAuth not configured for ${type}. Please set environment variables.`);
        setConnecting(false);
        return;
      }

      // Create provider and get auth URL
      const response = await addProvider(type);
      const { provider, auth_url, message } = response.data;

      if (auth_url) {
        // Store pending provider ID for polling
        setPendingProvider(provider.id);

        // Open OAuth popup
        const width = 600;
        const height = 700;
        const left = window.screenX + (window.outerWidth - width) / 2;
        const top = window.screenY + (window.outerHeight - height) / 2;

        const popup = window.open(
          auth_url,
          'oauth_popup',
          `width=${width},height=${height},left=${left},top=${top},menubar=no,toolbar=no,location=no,status=no`
        );

        // Check if popup was blocked
        if (!popup || popup.closed) {
          setError('Popup was blocked. Please allow popups for this site and try again.');
          setConnecting(false);
          setPendingProvider(null);
          return;
        }

        // Monitor popup close
        const checkPopup = setInterval(() => {
          if (popup.closed) {
            clearInterval(checkPopup);
            // The polling effect will handle checking for success
          }
        }, 500);
      } else if (message) {
        // OAuth not available, show message
        setError(message);
        setConnecting(false);
      } else {
        // Provider created but no auth needed (e.g., iCloud with credentials in env)
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
    if (step === 1 && selectedProviders.length > 0) {
      setStep(2);
    } else if (step === 2) {
      setStep(3);
    }
  };

  const handleComplete = () => {
    onComplete();
  };

  const isProviderConfigured = (type) => {
    return oauthStatus[type]?.configured !== false;
  };

  return (
    <div className="setup-wizard">
      <div className="wizard-header">
        <h1>Welcome to CloudUnify</h1>
        <p>Unify your cloud storage into one virtual drive</p>
      </div>

      <div className="wizard-progress">
        <div className={`step ${step >= 1 ? 'active' : ''}`}>1. Select Providers</div>
        <div className={`step ${step >= 2 ? 'active' : ''}`}>2. Connect</div>
        <div className={`step ${step >= 3 ? 'active' : ''}`}>3. Ready</div>
      </div>

      {step === 1 && (
        <div className="wizard-content">
          <h2>Select your cloud storage providers</h2>
          <p>Choose which services you want to unify</p>

          <div className="provider-options">
            {providerOptions.map((provider) => {
              const isConfigured = isProviderConfigured(provider.type);
              const isConnected = connectedProviders.includes(provider.type);

              return (
                <div
                  key={provider.type}
                  className={`provider-option ${selectedProviders.includes(provider.type) ? 'selected' : ''} ${!isConfigured ? 'disabled' : ''} ${isConnected ? 'connected' : ''}`}
                  onClick={() => !isConnected && isConfigured && toggleProvider(provider.type)}
                >
                  <span className="provider-icon">{provider.icon}</span>
                  <div className="provider-details">
                    <h3>{provider.name}</h3>
                    <p>{provider.description}</p>
                    <span className="quota-badge">{provider.quota}</span>
                    {!isConfigured && (
                      <span className="not-configured">OAuth not configured</span>
                    )}
                    {isConnected && (
                      <span className="already-connected">Already connected</span>
                    )}
                  </div>
                  <div className="checkbox">
                    {isConnected ? '✓' : selectedProviders.includes(provider.type) ? '✓' : ''}
                  </div>
                </div>
              );
            })}
          </div>

          <div className="wizard-actions">
            <button
              className="btn btn-primary"
              onClick={handleNext}
              disabled={selectedProviders.length === 0 && connectedProviders.length === 0}
            >
              {connectedProviders.length > 0 && selectedProviders.length === 0 ? 'Continue with existing' : 'Continue'}
            </button>
          </div>
        </div>
      )}

      {step === 2 && (
        <div className="wizard-content">
          <h2>Connect your accounts</h2>
          <p>Sign in to each service to grant CloudUnify access</p>

          {error && <div className="error">{error}</div>}

          <div className="connect-list">
            {selectedProviders.map((type) => {
              const provider = providerOptions.find((p) => p.type === type);
              const isConnected = connectedProviders.includes(type);
              const isPending = pendingProvider && connecting;

              return (
                <div key={type} className="connect-item">
                  <span className="provider-icon">{provider.icon}</span>
                  <span className="provider-name">{provider.name}</span>
                  {isConnected ? (
                    <span className="status connected">✓ Connected</span>
                  ) : (
                    <button
                      className="btn btn-secondary"
                      onClick={() => connectProvider(type)}
                      disabled={connecting}
                    >
                      {isPending ? 'Waiting for auth...' : connecting ? 'Connecting...' : 'Connect'}
                    </button>
                  )}
                </div>
              );
            })}
          </div>

          <div className="wizard-actions">
            <button className="btn" onClick={() => setStep(1)}>
              Back
            </button>
            <button
              className="btn btn-primary"
              onClick={handleNext}
              disabled={connectedProviders.length === 0 && selectedProviders.some(t => !connectedProviders.includes(t))}
            >
              {connectedProviders.length > 0 ? 'Continue' : 'Skip for now'}
            </button>
          </div>
        </div>
      )}

      {step === 3 && (
        <div className="wizard-content">
          <div className="success-icon">🎉</div>
          <h2>You're all set!</h2>
          <p>
            CloudUnify is ready to use. Your unified storage is mounted at{' '}
            <code>~/CloudUnify</code>
          </p>

          {connectedProviders.length > 0 && (
            <div className="summary">
              <h3>Connected Providers</h3>
              <ul>
                {connectedProviders.map((type) => {
                  const provider = providerOptions.find((p) => p.type === type);
                  return provider ? (
                    <li key={type}>
                      {provider.icon} {provider.name}
                    </li>
                  ) : null;
                })}
              </ul>
            </div>
          )}

          {connectedProviders.length === 0 && (
            <div className="summary warning">
              <p>No providers connected yet. You can add them later from Settings.</p>
            </div>
          )}

          <div className="wizard-actions">
            <button className="btn btn-primary" onClick={handleComplete}>
              Go to Dashboard
            </button>
          </div>
        </div>
      )}
    </div>
  );
}

export default SetupWizard;
