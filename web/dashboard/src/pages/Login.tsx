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
    <div className="min-h-screen flex items-center justify-center bg-gradient-to-br from-slate-900 via-slate-800 to-slate-900">
      <div className="w-full max-w-md">
        <div className="text-center mb-8">
          <img src="/fibratus.svg" alt="Fibratus" className="h-14 w-14 mx-auto" />
          <h1 className="mt-4 text-2xl font-bold text-white">
            Fibratus <span className="font-normal text-white/70">Fleet</span>
          </h1>
          <p className="mt-2 text-sm text-slate-400">
            {totpRequired ? 'Enter your authenticator code' : 'Sign in to your account'}
          </p>
        </div>

        <div className="rounded-2xl bg-white/5 backdrop-blur border border-white/10 p-8 shadow-2xl">
          <form onSubmit={handleSubmit}>
            {error && (
              <div className="mb-6 rounded-lg bg-red-500/10 border border-red-500/20 px-4 py-3 text-sm text-red-400">
                {error}
              </div>
            )}

            {!totpRequired ? (
              <div className="space-y-5">
                <div>
                  <label className="block text-sm font-medium text-slate-300 mb-1.5">Email</label>
                  <input
                    type="email"
                    value={email}
                    onChange={(e) => setEmail(e.target.value)}
                    required
                    className="rounded-lg bg-white/5 border border-white/10 text-white placeholder-slate-500 px-4 py-3 text-sm focus:border-fibratus-500 focus:ring-1 focus:ring-fibratus-500 focus:outline-none w-full"
                    placeholder="admin@example.com"
                  />
                </div>
                <div>
                  <label className="block text-sm font-medium text-slate-300 mb-1.5">Password</label>
                  <input
                    type="password"
                    value={password}
                    onChange={(e) => setPassword(e.target.value)}
                    required
                    className="rounded-lg bg-white/5 border border-white/10 text-white placeholder-slate-500 px-4 py-3 text-sm focus:border-fibratus-500 focus:ring-1 focus:ring-fibratus-500 focus:outline-none w-full"
                    placeholder="Enter your password"
                  />
                </div>
              </div>
            ) : (
              <div className="space-y-5">
                <p className="text-sm text-slate-400">
                  Enter the 6-digit code from your authenticator app.
                </p>
                <div>
                  <label className="block text-sm font-medium text-slate-300 mb-1.5">Authentication Code</label>
                  <input
                    type="text"
                    inputMode="numeric"
                    pattern="[0-9]{6}"
                    maxLength={6}
                    value={totpCode}
                    onChange={(e) => setTotpCode(e.target.value.replace(/\D/g, '').slice(0, 6))}
                    required
                    autoFocus
                    className="rounded-lg bg-white/5 border border-white/10 text-white placeholder-slate-500 px-4 py-3 text-sm text-center font-mono tracking-widest focus:border-fibratus-500 focus:ring-1 focus:ring-fibratus-500 focus:outline-none w-full"
                    placeholder="000000"
                  />
                </div>
              </div>
            )}

            <button
              type="submit"
              disabled={loading || (totpRequired && totpCode.length !== 6)}
              className="mt-6 w-full rounded-lg bg-fibratus-600 py-3 text-sm font-semibold text-white hover:bg-fibratus-700 focus:outline-none focus:ring-2 focus:ring-fibratus-500 focus:ring-offset-2 focus:ring-offset-slate-900 disabled:opacity-50 transition-colors"
            >
              {loading ? 'Verifying...' : totpRequired ? 'Verify Code' : 'Sign In'}
            </button>

            {totpRequired ? (
              <button
                type="button"
                onClick={() => { setTotpRequired(false); setTotpCode(''); setError('') }}
                className="mt-4 w-full text-center text-sm text-slate-500 hover:text-slate-300 transition-colors"
              >
                Back to login
              </button>
            ) : (
              <p className="mt-6 text-center text-sm text-slate-500">
                Don't have an account?{' '}
                <Link to="/signup" className="font-medium text-fibratus-400 hover:text-fibratus-300 transition-colors">
                  Create one
                </Link>
              </p>
            )}
          </form>
        </div>
      </div>
    </div>
  )
}
