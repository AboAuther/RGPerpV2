import { createBrowserRouter, Navigate } from 'react-router-dom';
import { AppShell } from '../shared/components';
import { ProtectedOutlet } from '../shared/auth';
import { AdminConfigsPage, AdminDashboardPage, AdminLiquidationsPage, AdminWithdrawalsPage } from '../pages/admin/AdminPages';
import { ExplorerPage } from '../pages/explorer/ExplorerPage';
import { FillsHistoryPage, FundingHistoryPage, OrdersHistoryPage, TransfersHistoryPage } from '../pages/history/HistoryPages';
import { LandingPage } from '../pages/landing/LandingPage';
import { LoginPage } from '../pages/login/LoginPage';
import { PortfolioPage } from '../pages/portfolio/PortfolioPage';
import { TradePage } from '../pages/trade/TradePage';
import { DepositPage } from '../pages/wallet/DepositPage';
import { WithdrawPage } from '../pages/wallet/WithdrawPage';

export const router = createBrowserRouter([
  {
    path: '/',
    element: <LandingPage />,
  },
  {
    path: '/login',
    element: <LoginPage />,
  },
  {
    element: <ProtectedOutlet />,
    children: [
      {
        element: <AppShell />,
        children: [
          { path: '/portfolio', element: <PortfolioPage /> },
          { path: '/trade', element: <TradePage /> },
          { path: '/wallet/deposit', element: <DepositPage /> },
          { path: '/wallet/withdraw', element: <WithdrawPage /> },
          { path: '/history/orders', element: <OrdersHistoryPage /> },
          { path: '/history/fills', element: <FillsHistoryPage /> },
          { path: '/history/funding', element: <FundingHistoryPage /> },
          { path: '/history/transfers', element: <TransfersHistoryPage /> },
          { path: '/explorer', element: <ExplorerPage /> },
          { path: '/admin/dashboard', element: <AdminDashboardPage /> },
          { path: '/admin/withdrawals', element: <AdminWithdrawalsPage /> },
          { path: '/admin/configs', element: <AdminConfigsPage /> },
          { path: '/admin/liquidations', element: <AdminLiquidationsPage /> },
          { path: '*', element: <Navigate replace to="/portfolio" /> },
        ],
      },
    ],
  },
  {
    path: '*',
    element: <Navigate replace to="/" />,
  },
]);
