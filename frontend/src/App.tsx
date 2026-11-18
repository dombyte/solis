
import { BrowserRouter, Routes, Route, Navigate } from 'react-router-dom';
import { lazy, Suspense } from 'react';
import { MobileNav } from './components/layout/MobileNav';
import { DesktopNav } from './components/layout/DesktopNav';
import { MobileHeader } from './components/layout/MobileHeader';
import { UpdateBanner } from './components/layout/UpdateBanner';
import { PageSkeleton } from './components/layout/PageSkeleton';
import { DocumentTitle } from './components/layout/DocumentTitle';
import { FaultNotifier } from './components/layout/FaultNotifier';
import { useCompactLayout } from './hooks/useMobile';


const Dashboard = lazy(() => import('./pages/Dashboard').then(m => ({ default: m.Dashboard })));
const History = lazy(() => import('./pages/History').then(m => ({ default: m.History })));
const Reports = lazy(() => import('./pages/Reports').then(m => ({ default: m.Reports })));
const Status = lazy(() => import('./pages/Status').then(m => ({ default: m.Status })));
const Info = lazy(() => import('./pages/Info').then(m => ({ default: m.Info })));

export function App() {
  const isMobile = useCompactLayout();

  return (
        <BrowserRouter>
          <DocumentTitle />
          <FaultNotifier />
          {/* Desktop: the shell is exactly viewport-tall and only <main> scrolls (its overflow-x
              makes it a scroll container), so the sidebar never moves. Mobile: the page scrolls. */}
          <div className={`bg-background flex ${isMobile ? 'min-h-screen flex-col mobile-nav-spacer' : 'h-screen flex-row overflow-hidden'} w-full max-w-[100vw] overflow-x-hidden`}>
            {!isMobile && <DesktopNav />}
            <div className="flex flex-col flex-1 w-full relative overflow-x-hidden">
              {isMobile && <MobileHeader />}
              <main className="flex-1 w-full overflow-x-hidden pt-1">
                <Suspense fallback={<PageSkeleton />}>
                  <Routes>
                    <Route path="/" element={<Dashboard />} />
                    <Route path="/dashboard" element={<Dashboard />} />
                    <Route path="/history" element={<History />} />
                    <Route path="/reports" element={<Reports />} />
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
            <UpdateBanner />
          </div>
        </BrowserRouter>
  );
}
