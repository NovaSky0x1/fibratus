import { useState } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { api, setSession, type AuthResponse } from '../lib/api'

export default function Login() {
  const navigate = useNavigate()
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(false)

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    setError('')
    setLoading(true)

    try {
      const res = await api.login({ email, password })
      if (res.error) {
        setError(res.error.message)
        return
      }
      const data = res.data as AuthResponse
      if (data.token) {
        // After login, fetch orgs and use the first one
        setSession(data.token, '')
        const orgsRes = await api.getOrganizations()
        const orgs = orgsRes.data as { id: string }[] | undefined
        if (orgs && orgs.length > 0) {
          setSession(data.token, orgs[0].id)
        }
        navigate('/')
      }
    } catch {
      setError('Failed to connect to server')
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="flex min-h-screen items-center justify-center bg-gray-50">
      <div className="w-full max-w-md">
        <div className="text-center mb-8">
          <h1 className="text-3xl font-bold text-gray-900">
            <span className="text-fibratus-600">Fibratus</span> Fleet
          </h1>
          <p className="mt-2 text-sm text-gray-500">Sign in to your account</p>
        </div>

        <form onSubmit={handleSubmit} className="rounded-xl border border-gray-200 bg-white p-8 shadow-sm">
          {error && (
            <div className="mb-4 rounded-lg bg-red-50 border border-red-200 px-4 py-3 text-sm text-red-700">
              {error}
            </div>
          )}

          <div className="space-y-4">
            <div>
              <label className="block text-sm font-medium text-gray-700">Email</label>
              <input
                type="email"
                value={email}
                onChange={(e) => setEmail(e.target.value)}
                required
                className="mt-1 w-full rounded-lg border border-gray-300 px-4 py-2.5 text-sm focus:border-fibratus-500 focus:outline-none focus:ring-1 focus:ring-fibratus-500"
                placeholder="admin@example.com"
              />
            </div>
            <div>
              <label className="block text-sm font-medium text-gray-700">Password</label>
              <input
                type="password"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                required
                className="mt-1 w-full rounded-lg border border-gray-300 px-4 py-2.5 text-sm focus:border-fibratus-500 focus:outline-none focus:ring-1 focus:ring-fibratus-500"
              />
            </div>
          </div>

          <button
            type="submit"
            disabled={loading}
            className="mt-6 w-full rounded-lg bg-fibratus-600 px-4 py-2.5 text-sm font-medium text-white hover:bg-fibratus-700 disabled:opacity-50"
          >
            {loading ? 'Signing in...' : 'Sign in'}
          </button>

          <p className="mt-4 text-center text-sm text-gray-500">
            Don't have an account?{' '}
            <Link to="/signup" className="font-medium text-fibratus-600 hover:text-fibratus-500">
              Create one
            </Link>
          </p>
        </form>
      </div>
    </div>
  )
}
