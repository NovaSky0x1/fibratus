import { Routes, Route, Navigate } from 'react-router-dom'
import { isAuthenticated } from './lib/api'
import Layout from './components/Layout'
import Login from './pages/Login'
import Signup from './pages/Signup'
import Overview from './pages/Overview'
import Agents from './pages/Agents'
import Detections from './pages/Detections'

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
              </Routes>
            </Layout>
          </ProtectedRoute>
        }
      />
    </Routes>
  )
}
