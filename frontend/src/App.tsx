
import { BrowserRouter, Routes, Route, Navigate } from 'react-router-dom';
import { lazy, Suspense } from 'react';
import { MobileNav } from './components/layout/MobileNav';
import { DesktopNav } from './components/layout/DesktopNav';
import { MobileHeader } from './components/layout/MobileHeader';
import { ToastContainer } from './components/layout/ToastContainer';
import { UpdateBanner } from './components/layout/UpdateBanner';
import { LoadingScreen } from './components/layout/LoadingScreen';

import { ToastProvider } from './components/ui/toast';

const Dashboard = lazy(() => import('./pages/Dashboard').then(m => ({ default: m.Dashboard })));
const History = lazy(() => import('./pages/History').then(m => ({ default: m.History })));
const Status = lazy(() => import('./pages/Status').then(m => ({ default: m.Status })));
const Info = lazy(() => import('./pages/Info').then(m => ({ default: m.Info })));

export function App() {
  return (
    <ToastProvider>
        <BrowserRouter>
          <div className="min-h-screen bg-background flex flex-col md:flex-row w-full max-w-[100vw] overflow-x-hidden pb-24 md:pb-0">
            <DesktopNav />
            <div className="flex flex-col flex-1 w-full relative overflow-x-hidden">
              <MobileHeader />
              <main className="flex-1 w-full overflow-x-hidden pt-1 md:pt-0">
                <Suspense fallback={<LoadingScreen message="Loading page..." fullPage={false} />}>
                  <Routes>
                    <Route path="/" element={<Dashboard />} />
                    <Route path="/dashboard" element={<Dashboard />} />
                    <Route path="/history" element={<History />} />
                    <Route path="/status" element={<Status />} />
                    <Route path="/info" element={<Info />} />
                    <Route path="*" element={
                      window.location.pathname.startsWith('/api/') ||
                      window.location.pathname.startsWith('/docs') ||
                      window.location.pathname.startsWith('/health') ||
                      window.location.pathname === '/metrics' ||
                      window.location.pathname.startsWith('/ws')
                        ? null
                        : <Navigate to="/" replace />
                    } />
                  </Routes>
                </Suspense>
              </main>
              <MobileNav />
            </div>
            <ToastContainer />
            <UpdateBanner />
          </div>
        </BrowserRouter>
      </ToastProvider>
  );
}
