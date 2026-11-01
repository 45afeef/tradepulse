import { Navigate, Route, Routes, useLocation } from 'react-router-dom';
import type { ReactNode } from 'react';
import { useAuth } from './auth/AuthContext';
import { AppShell } from './components/AppShell';
import { Spinner } from './ui';
import { LoginPage, RegisterPage } from './pages/Auth';
import { FeedPage } from './pages/Feed';
import { AlertsPage } from './pages/Alerts';
import { FollowingPage } from './pages/Following';
import { DeliveryPage } from './pages/Delivery';
import { SettingsPage } from './pages/Settings';
import { WelcomePage } from './pages/Welcome';
import { AdminOverview } from './pages/admin/Overview';
import { AdminSources } from './pages/admin/Sources';
import { AdminInvites } from './pages/admin/Invites';
import { AdminMembers } from './pages/admin/Members';
import { AdminAudit } from './pages/admin/Audit';
import { AdminPublish } from './pages/admin/Publish';

function FullPageSpinner() { return <div className="auth-wrap"><Spinner label="Loading your workspace" /></div>; }

function RequireAuth({ children }: { children: ReactNode }) {
  const { status } = useAuth();
  const loc = useLocation();
  if (status === 'loading') return <FullPageSpinner />;
  if (status === 'anon') return <Navigate to="/login" replace state={{ from: loc.pathname + loc.search }} />;
  return <>{children}</>;
}

function RequireAdmin({ children }: { children: ReactNode }) {
  const { isAdmin } = useAuth();
  return isAdmin ? <>{children}</> : <Navigate to="/feed" replace />;
}

function GuestOnly({ children }: { children: ReactNode }) {
  const { status } = useAuth();
  if (status === 'loading') return <FullPageSpinner />;
  return status === 'authed' ? <Navigate to="/feed" replace /> : <>{children}</>;
}

export function App() {
  return (
    <Routes>
      <Route path="/login" element={<GuestOnly><LoginPage /></GuestOnly>} />
      <Route path="/register" element={<GuestOnly><RegisterPage /></GuestOnly>} />
      <Route element={<RequireAuth><AppShell /></RequireAuth>}>
        <Route path="/feed" element={<FeedPage />} />
        <Route path="/welcome" element={<WelcomePage />} />
        <Route path="/alerts" element={<AlertsPage />} />
        <Route path="/following" element={<FollowingPage />} />
        <Route path="/delivery" element={<DeliveryPage />} />
        <Route path="/settings" element={<SettingsPage />} />
        <Route path="/admin" element={<RequireAdmin><AdminOverview /></RequireAdmin>} />
        <Route path="/admin/sources" element={<RequireAdmin><AdminSources /></RequireAdmin>} />
        <Route path="/admin/invites" element={<RequireAdmin><AdminInvites /></RequireAdmin>} />
        <Route path="/admin/members" element={<RequireAdmin><AdminMembers /></RequireAdmin>} />
        <Route path="/admin/audit" element={<RequireAdmin><AdminAudit /></RequireAdmin>} />
        <Route path="/admin/publish" element={<RequireAdmin><AdminPublish /></RequireAdmin>} />
      </Route>
      <Route path="*" element={<Navigate to="/feed" replace />} />
    </Routes>
  );
}
