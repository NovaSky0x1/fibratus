import { Routes, Route, Navigate } from 'react-router-dom'
import { isAuthenticated } from './lib/api'
import Layout from './components/Layout'
import Login from './pages/Login'
import Signup from './pages/Signup'
import Overview from './pages/Overview'
import Agents from './pages/Agents'
import Detections from './pages/Detections'
import Rules from './pages/Rules'
import Macros from './pages/Macros'
import Events from './pages/Events'
import AuditLog from './pages/AuditLog'
import Settings from './pages/Settings'
import Management from './pages/Management'
import Admin from './pages/Admin'

function ProtectedRoute({ children }: { children: React.ReactNode }) {
  if (!isAuthenticated()) {
    return <Navigate to="/login" replace />
  }
  return <>{children}</>
}

export default function App() {
  return (
    <Routes>
      <Route path="/login" element={<Login />} />
      <Route path="/signup" element={<Signup />} />
      <Route
        path="/*"
        element={
          <ProtectedRoute>
            <Layout>
              <Routes>
                <Route path="/" element={<Overview />} />
                <Route path="/agents" element={<Agents />} />
                <Route path="/detections" element={<Detections />} />
                <Route path="/rules" element={<Rules />} />
                <Route path="/macros" element={<Macros />} />
                <Route path="/events" element={<Events />} />
                <Route path="/audit-log" element={<AuditLog />} />
                <Route path="/settings" element={<Settings />} />
                <Route path="/management" element={<Management />} />
                <Route path="/admin" element={<Admin />} />
              </Routes>
            </Layout>
          </ProtectedRoute>
        }
      />
    </Routes>
  )
}
