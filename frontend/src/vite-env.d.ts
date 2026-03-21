/// <reference types="vite/client" />

interface EthereumProvider {
  isMetaMask?: boolean;
  request(args: { method: string; params?: unknown[] | object }): Promise<unknown>;
}

interface Window {
  ethereum?: EthereumProvider;
}
