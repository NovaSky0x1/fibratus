import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { api, type User } from '../lib/api'

const passwordRules = [
  { label: '12+ characters', test: (p: string) => p.length >= 12 },
  { label: 'Uppercase letter', test: (p: string) => /[A-Z]/.test(p) },
  { label: 'Lowercase letter', test: (p: string) => /[a-z]/.test(p) },
  { label: 'Digit', test: (p: string) => /\d/.test(p) },
  { label: 'Special character', test: (p: string) => /[^A-Za-z0-9]/.test(p) },
]

const cardClass = 'rounded-xl border border-gray-200 dark:border-slate-700 bg-white dark:bg-slate-800 p-6 shadow-sm dark:shadow-slate-900/50'
const labelClass = 'block text-sm font-medium text-gray-700 dark:text-slate-300'
const inputClass = 'mt-1 w-full rounded-lg border border-gray-300 dark:border-slate-600 bg-white dark:bg-slate-700 px-4 py-2.5 text-sm text-gray-900 dark:text-slate-100 placeholder-gray-400 dark:placeholder-slate-500 focus:border-fibratus-500 focus:outline-none focus:ring-1 focus:ring-fibratus-500'
const btnPrimary = 'rounded-lg bg-fibratus-600 px-4 py-2.5 text-sm font-medium text-white hover:bg-fibratus-700 disabled:opacity-50'

export default function Profile() {
  const queryClient = useQueryClient()

  // ── Current user query ────────────────────────────────────
  const { data: userData, isLoading } = useQuery({
    queryKey: ['current-user'],
    queryFn: () => api.getCurrentUser(),
  })
  const user = userData?.data as User | undefined

  // ── Personal information state ────────────────────────────
  const [name, setName] = useState('')
  const [nameInitialized, setNameInitialized] = useState(false)
  const [profileMsg, setProfileMsg] = useState('')
  const [profileError, setProfileError] = useState('')

  // Initialize name from user data once loaded
  if (user && !nameInitialized) {
    setName(user.name || '')
    setNameInitialized(true)
  }

  const profileMutation = useMutation({
    mutationFn: () => api.updateMyProfile({ name }),
    onSuccess: (res) => {
      if (res.error) { setProfileError(res.error.message); return }
      setProfileMsg('Profile updated successfully.')
      setProfileError('')
      queryClient.invalidateQueries({ queryKey: ['current-user'] })
    },
    onError: () => setProfileError('Failed to update profile'),
  })

  // ── Password change state ─────────────────────────────────
  const [currentPassword, setCurrentPassword] = useState('')
  const [newPassword, setNewPassword] = useState('')
  const [confirmPassword, setConfirmPassword] = useState('')
  const [passwordMsg, setPasswordMsg] = useState('')
  const [passwordError, setPasswordError] = useState('')

  const passwordMutation = useMutation({
    mutationFn: () => api.changeMyPassword({ current_password: currentPassword, new_password: newPassword }),
    onSuccess: (res) => {
      if (res.error) { setPasswordError(res.error.message); return }
      setPasswordMsg('Password changed successfully.')
      setPasswordError('')
      setCurrentPassword('')
      setNewPassword('')
      setConfirmPassword('')
    },
    onError: () => setPasswordError('Failed to change password'),
  })

  const passwordsMatch = newPassword === confirmPassword
  const allRulesPass = passwordRules.every(r => r.test(newPassword))
  const canChangePassword = currentPassword && newPassword && confirmPassword && passwordsMatch && allRulesPass

  // ── TOTP 2FA state ────────────────────────────────────────
  const { data: totpData } = useQuery({
    queryKey: ['totp-status'],
    queryFn: () => api.getTOTPStatus(),
  })
  const totpEnabled = (totpData?.data as { enabled: boolean } | undefined)?.enabled ?? false

  const [setupStep, setSetupStep] = useState<'idle' | 'setup' | 'verify' | 'done'>('idle')
  const [totpSecret, setTotpSecret] = useState('')
  const [verifyCode, setVerifyCode] = useState('')
  const [recoveryCodes, setRecoveryCodes] = useState<string[]>([])
  const [setupError, setSetupError] = useState('')
  const [qrDataUrl, setQrDataUrl] = useState('')
  const [showDisable, setShowDisable] = useState(false)
  const [disablePassword, setDisablePassword] = useState('')
  const [disableError, setDisableError] = useState('')

  const setupMutation = useMutation({
    mutationFn: () => api.setupTOTP(),
    onSuccess: (res) => {
      if (res.error) { setSetupError(res.error.message); return }
      const data = res.data as { secret: string; uri: string; qr: string } | undefined
      if (data) {
        setTotpSecret(data.secret)
        setQrDataUrl(data.qr || '')
        setSetupStep('verify')
      }
    },
    onError: () => setSetupError('Failed to start 2FA setup'),
  })

  const verifyMutation = useMutation({
    mutationFn: () => api.verifyTOTP(verifyCode),
    onSuccess: (res) => {
      if (res.error) { setSetupError(res.error.message); return }
      const data = res.data as { enabled: boolean; recovery_codes: string[] } | undefined
      if (data) {
        setRecoveryCodes(data.recovery_codes || [])
        setSetupStep('done')
        queryClient.invalidateQueries({ queryKey: ['totp-status'] })
      }
    },
    onError: () => setSetupError('Failed to verify code'),
  })

  const disableMutation = useMutation({
    mutationFn: () => api.disableTOTP(disablePassword),
    onSuccess: (res) => {
      if (res.error) { setDisableError(res.error.message); return }
      setShowDisable(false)
      setDisablePassword('')
      setDisableError('')
      queryClient.invalidateQueries({ queryKey: ['totp-status'] })
    },
    onError: () => setDisableError('Failed to disable 2FA'),
  })

  const resetSetup = () => {
    setSetupStep('idle')
    setTotpSecret('')
    setQrDataUrl('')
    setVerifyCode('')
    setRecoveryCodes([])
    setSetupError('')
  }

  // ── Loading state ─────────────────────────────────────────
  if (isLoading) {
    return (
      <div className="flex items-center justify-center py-24">
        <p className="text-sm text-gray-400 dark:text-slate-500">Loading profile...</p>
      </div>
    )
  }

  return (
    <div>
      <h1 className="text-2xl font-bold text-gray-900 dark:text-white">Profile</h1>
      <p className="mt-1 text-sm text-gray-500 dark:text-slate-400">Manage your account settings and security</p>

      <div className="mt-8 max-w-2xl space-y-8">
        {/* ── Personal Information ──────────────────────────── */}
        <div className={cardClass}>
          <h3 className="text-sm font-semibold text-gray-900 dark:text-slate-100">Personal Information</h3>
          <div className="mt-4 space-y-4">
            <div>
              <label className={labelClass}>Name</label>
              <input
                type="text"
                value={name}
                onChange={(e) => { setName(e.target.value); setProfileMsg(''); setProfileError('') }}
                className={inputClass}
                placeholder="Your name"
              />
            </div>
            <div>
              <label className={labelClass}>Email</label>
              <p className="mt-1 text-sm text-gray-600 dark:text-slate-400">{user?.email || '-'}</p>
            </div>
          </div>
          {profileError && (
            <div className="mt-4 rounded-lg border border-red-200 dark:border-red-800/50 bg-red-50 dark:bg-red-900/20 px-4 py-2 text-sm text-red-700 dark:text-red-400">{profileError}</div>
          )}
          {profileMsg && (
            <div className="mt-4 rounded-lg border border-emerald-200 dark:border-emerald-800/50 bg-emerald-50 dark:bg-emerald-900/20 px-4 py-2 text-sm text-emerald-700 dark:text-emerald-400">{profileMsg}</div>
          )}
          <div className="mt-4">
            <button
              onClick={() => { setProfileError(''); setProfileMsg(''); profileMutation.mutate() }}
              disabled={!name.trim() || profileMutation.isPending}
              className={btnPrimary}
            >
              {profileMutation.isPending ? 'Saving...' : 'Save'}
            </button>
          </div>
        </div>

        {/* ── Password ─────────────────────────────────────── */}
        <div className={cardClass}>
          <h3 className="text-sm font-semibold text-gray-900 dark:text-slate-100">Password</h3>
          <div className="mt-4 space-y-4">
            <div>
              <label className={labelClass}>Current Password</label>
              <input
                type="password"
                value={currentPassword}
                onChange={(e) => { setCurrentPassword(e.target.value); setPasswordMsg(''); setPasswordError('') }}
                className={inputClass}
                placeholder="Enter your current password"
              />
            </div>
            <div>
              <label className={labelClass}>New Password</label>
              <input
                type="password"
                value={newPassword}
                onChange={(e) => { setNewPassword(e.target.value); setPasswordMsg(''); setPasswordError('') }}
                className={inputClass}
                placeholder="Enter a new password"
              />
            </div>
            <div>
              <label className={labelClass}>Confirm New Password</label>
              <input
                type="password"
                value={confirmPassword}
                onChange={(e) => { setConfirmPassword(e.target.value); setPasswordMsg(''); setPasswordError('') }}
                className={inputClass}
                placeholder="Confirm your new password"
              />
              {confirmPassword && !passwordsMatch && (
                <p className="mt-1 text-xs text-red-600 dark:text-red-400">Passwords do not match</p>
              )}
            </div>

            {/* Password strength indicator */}
            {newPassword && (
              <div className="rounded-lg border border-gray-100 dark:border-slate-700 bg-gray-50 dark:bg-slate-900 p-3">
                <p className="text-xs font-medium text-gray-500 dark:text-slate-400 mb-2">Password requirements</p>
                <div className="grid grid-cols-2 gap-1.5">
                  {passwordRules.map((rule) => {
                    const passes = rule.test(newPassword)
                    return (
                      <div key={rule.label} className="flex items-center gap-1.5">
                        <span className={passes ? 'text-emerald-500' : 'text-gray-300 dark:text-slate-600'}>
                          {passes ? (
                            <svg className="h-3.5 w-3.5" viewBox="0 0 20 20" fill="currentColor"><path fillRule="evenodd" d="M16.707 5.293a1 1 0 010 1.414l-8 8a1 1 0 01-1.414 0l-4-4a1 1 0 011.414-1.414L8 12.586l7.293-7.293a1 1 0 011.414 0z" clipRule="evenodd" /></svg>
                          ) : (
                            <svg className="h-3.5 w-3.5" viewBox="0 0 20 20" fill="currentColor"><circle cx="10" cy="10" r="4" /></svg>
                          )}
                        </span>
                        <span className={'text-xs ' + (passes ? 'text-emerald-700 dark:text-emerald-400' : 'text-gray-400 dark:text-slate-500')}>{rule.label}</span>
                      </div>
                    )
                  })}
                </div>
              </div>
            )}
          </div>
          {passwordError && (
            <div className="mt-4 rounded-lg border border-red-200 dark:border-red-800/50 bg-red-50 dark:bg-red-900/20 px-4 py-2 text-sm text-red-700 dark:text-red-400">{passwordError}</div>
          )}
          {passwordMsg && (
            <div className="mt-4 rounded-lg border border-emerald-200 dark:border-emerald-800/50 bg-emerald-50 dark:bg-emerald-900/20 px-4 py-2 text-sm text-emerald-700 dark:text-emerald-400">{passwordMsg}</div>
          )}
          <div className="mt-4">
            <button
              onClick={() => { setPasswordError(''); setPasswordMsg(''); passwordMutation.mutate() }}
              disabled={!canChangePassword || passwordMutation.isPending}
              className={btnPrimary}
            >
              {passwordMutation.isPending ? 'Changing...' : 'Change Password'}
            </button>
          </div>
        </div>

        {/* ── Two-Factor Authentication ────────────────────── */}
        <div className={cardClass}>
          <div className="flex items-center justify-between">
            <div>
              <h3 className="text-sm font-semibold text-gray-900 dark:text-slate-100">Two-Factor Authentication</h3>
              <p className="mt-1 text-sm text-gray-500 dark:text-slate-400">
                {totpEnabled ? 'Your account is protected with 2FA.' : 'Add an extra layer of security to your account.'}
              </p>
            </div>
            <div className="flex items-center gap-3">
              <span className={'inline-flex items-center rounded-full px-2.5 py-0.5 text-xs font-medium ' +
                (totpEnabled ? 'text-emerald-700 bg-emerald-50 dark:text-emerald-400 dark:bg-emerald-900/30' : 'text-gray-500 bg-gray-100 dark:text-slate-400 dark:bg-slate-700')}>
                {totpEnabled ? 'Enabled' : 'Disabled'}
              </span>
              {!totpEnabled && setupStep === 'idle' && (
                <button
                  onClick={() => { setSetupError(''); setupMutation.mutate() }}
                  disabled={setupMutation.isPending}
                  className={btnPrimary}
                >
                  {setupMutation.isPending ? 'Setting up...' : 'Enable 2FA'}
                </button>
              )}
              {totpEnabled && !showDisable && (
                <button
                  onClick={() => setShowDisable(true)}
                  className="rounded-lg border border-red-300 dark:border-red-700 px-4 py-2 text-sm font-medium text-red-600 dark:text-red-400 hover:bg-red-50 dark:hover:bg-red-900/20"
                >
                  Disable 2FA
                </button>
              )}
            </div>
          </div>

          {setupError && (
            <div className="mt-4 rounded-lg border border-red-200 dark:border-red-800/50 bg-red-50 dark:bg-red-900/20 px-4 py-2 text-sm text-red-700 dark:text-red-400">{setupError}</div>
          )}

          {setupStep === 'verify' && (
            <div className="mt-4 rounded-lg border border-gray-200 dark:border-slate-700 bg-gray-50 dark:bg-slate-900 p-4 space-y-4">
              <div>
                <p className="text-sm font-medium text-gray-900 dark:text-slate-100">1. Scan with your authenticator app</p>
                <p className="mt-1 text-xs text-gray-500 dark:text-slate-400">Scan this QR code with your authenticator app (Google Authenticator, Authy, etc.)</p>
              </div>
              <div className="flex flex-col items-center py-2">
                {qrDataUrl ? (
                  <img src={qrDataUrl} alt="Scan with authenticator app" className="rounded-lg border border-gray-200 dark:border-slate-600 bg-white p-2" width={200} height={200} />
                ) : (
                  <div className="flex h-[200px] w-[200px] items-center justify-center rounded-lg border border-gray-200 dark:border-slate-600 bg-white text-xs text-gray-400">Generating QR code...</div>
                )}
              </div>
              <div>
                <label className="block text-xs font-medium text-gray-500 dark:text-slate-400 mb-1">Or enter the secret key manually</label>
                <div className="flex items-center gap-2">
                  <code className="flex-1 select-all rounded-lg border border-gray-200 dark:border-slate-600 bg-white dark:bg-slate-800 px-3 py-2 font-mono text-sm text-gray-900 dark:text-slate-100 break-all">{totpSecret}</code>
                  <button onClick={() => navigator.clipboard.writeText(totpSecret)} className="rounded-lg border border-gray-300 dark:border-slate-600 px-3 py-2 text-xs font-medium text-gray-600 dark:text-slate-300 hover:bg-gray-100 dark:hover:bg-slate-700">Copy</button>
                </div>
              </div>
              <div>
                <p className="text-sm font-medium text-gray-900 dark:text-slate-100">2. Enter the verification code</p>
                <div className="mt-2 flex items-center gap-3">
                  <input
                    type="text"
                    inputMode="numeric"
                    pattern="[0-9]{6}"
                    maxLength={6}
                    value={verifyCode}
                    onChange={(e) => setVerifyCode(e.target.value.replace(/\D/g, '').slice(0, 6))}
                    className="w-40 rounded-lg border border-gray-300 dark:border-slate-600 bg-white dark:bg-slate-700 px-3 py-2 text-sm text-center font-mono tracking-widest text-gray-900 dark:text-slate-100 focus:border-fibratus-500 focus:outline-none focus:ring-1 focus:ring-fibratus-500"
                    placeholder="000000"
                  />
                  <button
                    onClick={() => { setSetupError(''); verifyMutation.mutate() }}
                    disabled={verifyCode.length !== 6 || verifyMutation.isPending}
                    className={btnPrimary}
                  >
                    {verifyMutation.isPending ? 'Verifying...' : 'Verify'}
                  </button>
                  <button onClick={resetSetup} className="text-sm text-gray-500 dark:text-slate-400 hover:text-gray-700 dark:hover:text-slate-300">Cancel</button>
                </div>
              </div>
            </div>
          )}

          {setupStep === 'done' && recoveryCodes.length > 0 && (
            <div className="mt-4 rounded-lg border border-emerald-200 dark:border-emerald-800/50 bg-emerald-50 dark:bg-emerald-900/20 p-4 space-y-3">
              <div className="flex items-center justify-between">
                <div>
                  <p className="text-sm font-semibold text-emerald-800 dark:text-emerald-300">Two-factor authentication enabled</p>
                  <p className="text-xs text-emerald-600 dark:text-emerald-400">Save these recovery codes in a secure location. Each code can only be used once.</p>
                </div>
                <button onClick={() => navigator.clipboard.writeText(recoveryCodes.join('\n'))} className="rounded-lg bg-emerald-600 px-3 py-1.5 text-xs font-medium text-white hover:bg-emerald-700">Copy All</button>
              </div>
              <div className="grid grid-cols-2 gap-2">
                {recoveryCodes.map((code, i) => (
                  <code key={i} className="rounded border border-emerald-200 dark:border-emerald-700 bg-white dark:bg-slate-800 px-3 py-1.5 font-mono text-sm text-gray-900 dark:text-slate-100 text-center">{code}</code>
                ))}
              </div>
              <button onClick={resetSetup} className="text-sm font-medium text-emerald-700 dark:text-emerald-400 hover:text-emerald-800 dark:hover:text-emerald-300">Done</button>
            </div>
          )}

          {showDisable && (
            <div className="mt-4 rounded-lg border border-red-200 dark:border-red-800/50 bg-red-50 dark:bg-red-900/20 p-4 space-y-3">
              <p className="text-sm font-medium text-red-800 dark:text-red-300">Confirm password to disable 2FA</p>
              {disableError && (
                <div className="rounded-lg border border-red-300 dark:border-red-700 bg-white dark:bg-slate-800 px-3 py-2 text-sm text-red-700 dark:text-red-400">{disableError}</div>
              )}
              <div className="flex items-center gap-3">
                <input
                  type="password"
                  value={disablePassword}
                  onChange={(e) => setDisablePassword(e.target.value)}
                  placeholder="Enter your password"
                  className="w-64 rounded-lg border border-red-300 dark:border-red-700 bg-white dark:bg-slate-700 px-3 py-2 text-sm text-gray-900 dark:text-slate-100 focus:border-red-500 focus:outline-none focus:ring-1 focus:ring-red-500"
                />
                <button
                  onClick={() => { setDisableError(''); disableMutation.mutate() }}
                  disabled={!disablePassword || disableMutation.isPending}
                  className="rounded-lg bg-red-600 px-4 py-2 text-sm font-medium text-white hover:bg-red-700 disabled:opacity-50"
                >
                  {disableMutation.isPending ? 'Disabling...' : 'Disable 2FA'}
                </button>
                <button
                  onClick={() => { setShowDisable(false); setDisablePassword(''); setDisableError('') }}
                  className="text-sm text-gray-500 dark:text-slate-400 hover:text-gray-700 dark:hover:text-slate-300"
                >
                  Cancel
                </button>
              </div>
            </div>
          )}
        </div>
      </div>
    </div>
  )
}
