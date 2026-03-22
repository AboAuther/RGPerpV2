import { createContext, useContext, useEffect, useMemo, useState } from 'react';
import type { PropsWithChildren } from 'react';
import { api } from './api';
import type { SystemChainItem } from './domain';

interface SystemConfigContextValue {
  chains: SystemChainItem[];
  loading: boolean;
  error: unknown;
  refresh: () => Promise<void>;
  getChainById: (chainId: number) => SystemChainItem | undefined;
  localChain: SystemChainItem | undefined;
}

const SystemConfigContext = createContext<SystemConfigContextValue | null>(null);

export function SystemConfigProvider({ children }: PropsWithChildren) {
  const [chains, setChains] = useState<SystemChainItem[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<unknown>(null);

  async function refresh() {
    setLoading(true);
    setError(null);
    try {
      setChains(await api.system.getChains());
    } catch (loadError) {
      setChains([]);
      setError(loadError);
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    void refresh();
  }, []);

  const value = useMemo<SystemConfigContextValue>(() => {
    const byID = new Map<number, SystemChainItem>();
    for (const chain of chains) {
      byID.set(chain.chain_id, chain);
    }
    return {
      chains,
      loading,
      error,
      refresh,
      getChainById: (chainId) => byID.get(chainId),
      localChain: chains.find((chain) => chain.local_testnet),
    };
  }, [chains, loading, error]);

  return <SystemConfigContext.Provider value={value}>{children}</SystemConfigContext.Provider>;
}

export function useSystemConfig() {
  const context = useContext(SystemConfigContext);
  if (!context) {
    throw new Error('useSystemConfig must be used within SystemConfigProvider');
  }
  return context;
}
