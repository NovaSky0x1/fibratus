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
    <div className="flex min-h-screen items-center justify-center bg-gradient-to-br from-fibratus-900 to-fibratus-600">
      <div className="w-full max-w-md">
        <div className="text-center mb-8">
          <h1 className="text-3xl font-bold text-white">
            <span className="text-fibratus-300">Fibratus</span> Fleet
          </h1>
          <p className="mt-2 text-sm text-fibratus-200">
            {totpRequired ? 'Enter your authenticator code' : 'Sign in to your account'}
          </p>
        </div>

        <form onSubmit={handleSubmit} className="rounded-xl border border-gray-200 dark:border-slate-700 bg-white dark:bg-slate-800 p-8 shadow-sm dark:shadow-slate-900/50">
          {error && (
            <div className="mb-4 rounded-lg bg-red-50 border border-red-200 px-4 py-3 text-sm text-red-700">
              {error}
            </div>
          )}

          {!totpRequired ? (
            <div className="space-y-4">
              <div>
                <label className="block text-sm font-medium text-gray-700 dark:text-slate-300">Email</label>
                <input
                  type="email"
                  value={email}
                  onChange={(e) => setEmail(e.target.value)}
                  required
                  className="mt-1 w-full rounded-lg border border-gray-300 dark:border-slate-600 bg-white dark:bg-slate-700 px-4 py-2.5 text-sm text-gray-900 dark:text-slate-100 placeholder-gray-400 dark:placeholder-slate-500 focus:border-fibratus-500 focus:outline-none focus:ring-1 focus:ring-fibratus-500"
                  placeholder="admin@example.com"
                />
              </div>
              <div>
                <label className="block text-sm font-medium text-gray-700 dark:text-slate-300">Password</label>
                <input
                  type="password"
                  value={password}
                  onChange={(e) => setPassword(e.target.value)}
                  required
                  className="mt-1 w-full rounded-lg border border-gray-300 dark:border-slate-600 bg-white dark:bg-slate-700 px-4 py-2.5 text-sm text-gray-900 dark:text-slate-100 focus:border-fibratus-500 focus:outline-none focus:ring-1 focus:ring-fibratus-500"
                />
              </div>
            </div>
          ) : (
            <div className="space-y-4">
              <p className="text-sm text-gray-600 dark:text-slate-400">
                Enter the 6-digit code from your authenticator app.
              </p>
              <div>
                <label className="block text-sm font-medium text-gray-700 dark:text-slate-300">Authentication Code</label>
                <input
                  type="text"
                  inputMode="numeric"
                  pattern="[0-9]{6}"
                  maxLength={6}
                  value={totpCode}
                  onChange={(e) => setTotpCode(e.target.value.replace(/\D/g, '').slice(0, 6))}
                  required
                  autoFocus
                  className="mt-1 w-full rounded-lg border border-gray-300 dark:border-slate-600 bg-white dark:bg-slate-700 px-4 py-2.5 text-sm text-center font-mono tracking-widest text-gray-900 dark:text-slate-100 placeholder-gray-400 dark:placeholder-slate-500 focus:border-fibratus-500 focus:outline-none focus:ring-1 focus:ring-fibratus-500"
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
              className="mt-3 w-full text-center text-sm text-gray-500 dark:text-slate-400 hover:text-gray-700 dark:hover:text-slate-300"
            >
              Back to login
            </button>
          ) : (
            <p className="mt-4 text-center text-sm text-gray-500 dark:text-slate-400">
              Don't have an account?{' '}
              <Link to="/signup" className="font-medium text-fibratus-600 dark:text-fibratus-400 hover:text-fibratus-500">
                Create one
              </Link>
            </p>
          )}
        </form>
      </div>
    </div>
  )
}
