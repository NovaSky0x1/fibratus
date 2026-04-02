import { Routes, Route } from 'react-router-dom'
import Layout from './components/Layout'
import Overview from './pages/Overview'
import Agents from './pages/Agents'
import Detections from './pages/Detections'

export default function App() {
  return (
    <Layout>
      <Routes>
        <Route path="/" element={<Overview />} />
        <Route path="/agents" element={<Agents />} />
        <Route path="/detections" element={<Detections />} />
      </Routes>
    </Layout>
  )
}
