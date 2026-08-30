
import { BrowserRouter, Routes, Route, Navigate } from 'react-router-dom';
import { lazy, Suspense } from 'react';
import { MobileNav } from './components/layout/MobileNav';
import { DesktopNav } from './components/layout/DesktopNav';
import { MobileHeader } from './components/layout/MobileHeader';
import { ToastContainer } from './components/layout/ToastContainer';
import { UpdateBanner } from './components/layout/UpdateBanner';
import { LoadingScreen } from './components/layout/LoadingScreen';
import { useMobile } from './hooks/useMobile';

import { ToastProvider } from './components/ui/toast';

const Dashboard = lazy(() => import('./pages/Dashboard').then(m => ({ default: m.Dashboard })));
const History = lazy(() => import('./pages/History').then(m => ({ default: m.History })));
const Status = lazy(() => import('./pages/Status').then(m => ({ default: m.Status })));
const Info = lazy(() => import('./pages/Info').then(m => ({ default: m.Info })));

export function App() {
  const isMobile = useMobile();

  return (
    <ToastProvider>
        <BrowserRouter>
          <div className={`min-h-screen bg-background flex ${isMobile ? 'flex-col' : 'flex-row'} w-full max-w-[100vw] overflow-x-hidden ${isMobile ? 'pb-24' : 'pb-0'}`}>
            {!isMobile && <DesktopNav />}
            <div className="flex flex-col flex-1 w-full relative overflow-x-hidden">
              {isMobile && <MobileHeader />}
              <main className="flex-1 w-full overflow-x-hidden pt-1">
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
              {isMobile && <MobileNav />}
            </div>
            <ToastContainer />
            <UpdateBanner />
          </div>
        </BrowserRouter>
      </ToastProvider>
  );
}
