import { useState, useEffect } from 'react'
import { Link, useNavigate, useSearchParams } from 'react-router-dom'
import { api, setSession, isAuthenticated, type AuthResponse } from '../lib/api'
import { usePermissions } from '../contexts/PermissionContext'

export default function Login() {
  const navigate = useNavigate()
  const { refresh: refreshPermissions } = usePermissions()
  const [searchParams] = useSearchParams()
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(false)

  // TOTP verification (user already has 2FA)
  const [totpRequired, setTotpRequired] = useState(false)
  const [totpCode, setTotpCode] = useState('')

  // MFA setup (account requires 2FA, user hasn't set it up)
  const [mfaSetupRequired, setMfaSetupRequired] = useState(false)
  const [pendingToken, setPendingToken] = useState('')
  const [setupStep, setSetupStep] = useState<'qr' | 'verify' | 'done'>('qr')
  const [qrDataUrl, setQrDataUrl] = useState('')
  const [totpSecret, setTotpSecret] = useState('')
  const [verifyCode, setVerifyCode] = useState('')
  const [recoveryCodes, setRecoveryCodes] = useState<string[]>([])
  const [setupError, setSetupError] = useState('')
  const [setupLoading, setSetupLoading] = useState(false)

  // Auto-trigger MFA setup when coming from signup (already authenticated)
  useEffect(() => {
    if (searchParams.get('setup_2fa') === '1' && isAuthenticated()) {
      setMfaSetupRequired(true)
      setPendingToken(localStorage.getItem('fleet_token') || '')
      startMfaSetup('')
    }
  }, []) // eslint-disable-line react-hooks/exhaustive-deps

  const completeLogin = async (token: string) => {
    setSession(token, '')
    const orgsRes = await api.getOrganizations()
    const orgs = orgsRes.data as { id: string }[] | undefined
    if (orgs && orgs.length > 0) {
      setSession(token, orgs[0].id)
    }
    await refreshPermissions()
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
        // Check if MFA setup is required before completing login
        if (data.mfa_setup_required) {
          setPendingToken(data.token)
          setSession(data.token, '') // Need session to call TOTP APIs
          setMfaSetupRequired(true)
          // Start TOTP setup immediately
          startMfaSetup(data.token)
          return
        }
        await completeLogin(data.token)
      }
    } catch {
      setError('Failed to connect to server')
    } finally {
      setLoading(false)
    }
  }

  const startMfaSetup = async (_token: string) => {
    setSetupLoading(true)
    try {
      const res = await api.setupTOTP()
      if (res.error) { setSetupError(res.error.message); return }
      const data = res.data as { secret: string; uri: string; qr: string } | undefined
      if (data) {
        setTotpSecret(data.secret)
        setQrDataUrl(data.qr || '')
        setSetupStep('qr')
      }
    } catch {
      setSetupError('Failed to start 2FA setup')
    } finally {
      setSetupLoading(false)
    }
  }

  const handleVerifySetup = async () => {
    setSetupError('')
    setSetupLoading(true)
    try {
      const res = await api.verifyTOTP(verifyCode)
      if (res.error) { setSetupError(res.error.message); setSetupLoading(false); return }
      const data = res.data as { enabled: boolean; recovery_codes: string[] } | undefined
      if (data) {
        setRecoveryCodes(data.recovery_codes || [])
        setSetupStep('done')
      }
    } catch {
      setSetupError('Failed to verify code')
    } finally {
      setSetupLoading(false)
    }
  }

  const handleSetupComplete = async () => {
    setSession(pendingToken, '')
    const orgsRes = await api.getOrganizations()
    const orgs = orgsRes.data as { id: string }[] | undefined
    if (orgs && orgs.length > 0) {
      setSession(pendingToken, orgs[0].id)
    }
    await refreshPermissions()
    navigate('/')
  }

  const inputCls = 'rounded-lg bg-white/5 border border-white/10 text-white placeholder-slate-500 px-4 py-3 text-sm focus:border-fibratus-500 focus:ring-1 focus:ring-fibratus-500 focus:outline-none w-full'
  const btnCls = 'w-full rounded-lg bg-fibratus-600 py-3 text-sm font-semibold text-white hover:bg-fibratus-700 focus:outline-none focus:ring-2 focus:ring-fibratus-500 focus:ring-offset-2 focus:ring-offset-slate-900 disabled:opacity-50 transition-colors'

  return (
    <div className="min-h-screen flex items-center justify-center bg-gradient-to-br from-slate-900 via-slate-800 to-slate-900">
      <div className="w-full max-w-md">
        <div className="text-center mb-8">
          <img src="/fibratus.svg" alt="Fibratus" className="h-14 w-14 mx-auto" />
          <h1 className="mt-4 text-2xl font-bold text-white">
            Fibratus <span className="font-normal text-white/70">Fleet</span>
          </h1>
          <p className="mt-2 text-sm text-slate-400">
            {mfaSetupRequired
              ? 'Two-factor authentication is required'
              : totpRequired
                ? 'Enter your authenticator code'
                : 'Sign in to your account'}
          </p>
        </div>

        <div className="rounded-2xl bg-white/5 backdrop-blur border border-white/10 p-8 shadow-2xl">
          {/* ─── MFA Setup Flow ─── */}
          {mfaSetupRequired ? (
            <div className="space-y-5">
              {setupError && (
                <div className="rounded-lg bg-red-500/10 border border-red-500/20 px-4 py-3 text-sm text-red-400">{setupError}</div>
              )}

              {setupStep === 'qr' && (
                <>
                  <p className="text-sm text-slate-300">
                    Your organization requires two-factor authentication. Scan this QR code with your authenticator app.
                  </p>
                  <div className="flex flex-col items-center py-4">
                    {qrDataUrl ? (
                      <img src={qrDataUrl} alt="Scan with authenticator" className="rounded-lg border border-white/10 p-2 bg-white" width={200} height={200} />
                    ) : (
                      <div className="flex h-[200px] w-[200px] items-center justify-center rounded-lg border border-white/10 text-xs text-slate-500">
                        {setupLoading ? 'Generating...' : 'QR code unavailable'}
                      </div>
                    )}
                  </div>
                  <div>
                    <label className="block text-xs text-slate-400 mb-1">Or enter manually</label>
                    <div className="flex items-center gap-2">
                      <code className="flex-1 select-all rounded-lg bg-black/30 border border-white/10 px-3 py-2 font-mono text-xs text-emerald-400 break-all">
                        {totpSecret}
                      </code>
                      <button onClick={() => navigator.clipboard.writeText(totpSecret)} className="rounded-lg bg-white/10 px-3 py-2 text-xs text-slate-300 hover:bg-white/20">Copy</button>
                    </div>
                  </div>
                  <div>
                    <label className="block text-sm font-medium text-slate-300 mb-1.5">Verification Code</label>
                    <input
                      type="text"
                      inputMode="numeric"
                      pattern="[0-9]{6}"
                      maxLength={6}
                      value={verifyCode}
                      onChange={e => setVerifyCode(e.target.value.replace(/\D/g, '').slice(0, 6))}
                      className={inputCls + ' text-center font-mono tracking-widest'}
                      placeholder="000000"
                      autoFocus
                    />
                  </div>
                  <button
                    onClick={handleVerifySetup}
                    disabled={verifyCode.length !== 6 || setupLoading}
                    className={btnCls}
                  >
                    {setupLoading ? 'Verifying...' : 'Verify & Enable 2FA'}
                  </button>
                </>
              )}

              {setupStep === 'done' && (
                <>
                  <div className="rounded-lg bg-emerald-500/10 border border-emerald-500/20 px-4 py-3">
                    <p className="text-sm font-semibold text-emerald-400">Two-factor authentication enabled</p>
                    <p className="mt-1 text-xs text-emerald-400/70">Save these recovery codes — each can only be used once.</p>
                  </div>
                  <div className="grid grid-cols-2 gap-2">
                    {recoveryCodes.map((code, i) => (
                      <code key={i} className="rounded-lg bg-black/30 border border-white/10 px-3 py-1.5 font-mono text-xs text-slate-300 text-center">{code}</code>
                    ))}
                  </div>
                  <button
                    onClick={() => navigator.clipboard.writeText(recoveryCodes.join('\n'))}
                    className="w-full rounded-lg bg-white/10 py-2 text-xs text-slate-300 hover:bg-white/20"
                  >
                    Copy All Recovery Codes
                  </button>
                  <button onClick={handleSetupComplete} className={btnCls}>
                    Continue to Dashboard
                  </button>
                </>
              )}
            </div>
          ) : (
            /* ─── Normal Login / TOTP Flow ─── */
            <form onSubmit={handleSubmit}>
              {error && (
                <div className="mb-6 rounded-lg bg-red-500/10 border border-red-500/20 px-4 py-3 text-sm text-red-400">{error}</div>
              )}

              {!totpRequired ? (
                <div className="space-y-5">
                  <div>
                    <label className="block text-sm font-medium text-slate-300 mb-1.5">Email</label>
                    <input type="email" value={email} onChange={e => setEmail(e.target.value)} required className={inputCls} placeholder="admin@example.com" />
                  </div>
                  <div>
                    <label className="block text-sm font-medium text-slate-300 mb-1.5">Password</label>
                    <input type="password" value={password} onChange={e => setPassword(e.target.value)} required className={inputCls} placeholder="Enter your password" />
                  </div>
                </div>
              ) : (
                <div className="space-y-5">
                  <p className="text-sm text-slate-400">Enter the 6-digit code from your authenticator app.</p>
                  <div>
                    <label className="block text-sm font-medium text-slate-300 mb-1.5">Authentication Code</label>
                    <input
                      type="text" inputMode="numeric" pattern="[0-9]{6}" maxLength={6}
                      value={totpCode} onChange={e => setTotpCode(e.target.value.replace(/\D/g, '').slice(0, 6))}
                      required autoFocus className={inputCls + ' text-center font-mono tracking-widest'} placeholder="000000"
                    />
                  </div>
                </div>
              )}

              <button type="submit" disabled={loading || (totpRequired && totpCode.length !== 6)} className={'mt-6 ' + btnCls}>
                {loading ? 'Verifying...' : totpRequired ? 'Verify Code' : 'Sign In'}
              </button>

              {totpRequired ? (
                <button type="button" onClick={() => { setTotpRequired(false); setTotpCode(''); setError('') }}
                  className="mt-4 w-full text-center text-sm text-slate-500 hover:text-slate-300 transition-colors">
                  Back to login
                </button>
              ) : (
                <p className="mt-6 text-center text-sm text-slate-500">
                  Don't have an account?{' '}
                  <Link to="/signup" className="font-medium text-fibratus-400 hover:text-fibratus-300 transition-colors">Create one</Link>
                </p>
              )}
            </form>
          )}
        </div>
      </div>
    </div>
  )
}
