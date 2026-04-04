import { useState } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { api, setSession, type AuthResponse } from '../lib/api'

export default function Login() {
  const navigate = useNavigate()
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(false)
  const [totpRequired, setTotpRequired] = useState(false)
  const [totpCode, setTotpCode] = useState('')

  const completeLogin = async (token: string) => {
    setSession(token, '')
    const orgsRes = await api.getOrganizations()
    const orgs = orgsRes.data as { id: string }[] | undefined
    if (orgs && orgs.length > 0) {
      setSession(token, orgs[0].id)
    }
    navigate('/')
  }

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    setError('')
    setLoading(true)

    try {
      const payload: { email: string; password: string; totp_code?: string } = { email, password }
      if (totpRequired && totpCode) {
        payload.totp_code = totpCode
      }

      const res = await api.login(payload)
      if (res.error) {
        setError(res.error.message)
        return
      }
      const data = res.data as AuthResponse
      if (data.totp_required && !data.token) {
        setTotpRequired(true)
        return
      }
      if (data.token) {
        await completeLogin(data.token)
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
          <p className="mt-2 text-sm text-gray-500">
            {totpRequired ? 'Enter your authenticator code' : 'Sign in to your account'}
          </p>
        </div>

        <form onSubmit={handleSubmit} className="rounded-xl border border-gray-200 bg-white p-8 shadow-sm">
          {error && (
            <div className="mb-4 rounded-lg bg-red-50 border border-red-200 px-4 py-3 text-sm text-red-700">
              {error}
            </div>
          )}

          {!totpRequired ? (
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
          ) : (
            <div className="space-y-4">
              <p className="text-sm text-gray-600">
                Enter the 6-digit code from your authenticator app.
              </p>
              <div>
                <label className="block text-sm font-medium text-gray-700">Authentication Code</label>
                <input
                  type="text"
                  inputMode="numeric"
                  pattern="[0-9]{6}"
                  maxLength={6}
                  value={totpCode}
                  onChange={(e) => setTotpCode(e.target.value.replace(/\D/g, '').slice(0, 6))}
                  required
                  autoFocus
                  className="mt-1 w-full rounded-lg border border-gray-300 px-4 py-2.5 text-sm text-center font-mono tracking-widest focus:border-fibratus-500 focus:outline-none focus:ring-1 focus:ring-fibratus-500"
                  placeholder="000000"
                />
              </div>
            </div>
          )}

          <button
            type="submit"
            disabled={loading || (totpRequired && totpCode.length !== 6)}
            className="mt-6 w-full rounded-lg bg-fibratus-600 px-4 py-2.5 text-sm font-medium text-white hover:bg-fibratus-700 disabled:opacity-50"
          >
            {loading ? 'Verifying...' : totpRequired ? 'Verify Code' : 'Sign in'}
          </button>

          {totpRequired ? (
            <button
              type="button"
              onClick={() => { setTotpRequired(false); setTotpCode(''); setError('') }}
              className="mt-3 w-full text-center text-sm text-gray-500 hover:text-gray-700"
            >
              Back to login
            </button>
          ) : (
            <p className="mt-4 text-center text-sm text-gray-500">
              Don't have an account?{' '}
              <Link to="/signup" className="font-medium text-fibratus-600 hover:text-fibratus-500">
                Create one
              </Link>
            </p>
          )}
        </form>
      </div>
    </div>
  )
}
