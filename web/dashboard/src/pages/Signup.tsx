import { useState } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { api, setSession, type AuthResponse } from '../lib/api'

export default function Signup() {
  const navigate = useNavigate()
  const [form, setForm] = useState({
    account_name: '',
    org_name: '',
    email: '',
    name: '',
    password: '',
  })
  const [error, setError] = useState('')
  const [pending, setPending] = useState(false)
  const [loading, setLoading] = useState(false)

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    setError('')
    setLoading(true)

    try {
      const res = await api.signup(form)
      if (res.error) {
        setError(res.error.message)
        return
      }
      const data = res.data as AuthResponse & { pending_approval?: boolean }
      // Signup now requires admin approval — server returns pending_approval=true
      // and no token. Show a confirmation screen instead of routing into the app.
      if (data.pending_approval) {
        setPending(true)
        return
      }
      if (data.token) {
        // Set session, then redirect to login for mandatory 2FA setup
        if (data.mfa_setup_required) {
          setSession(data.token, data.org_id || '')
          // Store org_id from signup response
          if (data.org_id) setSession(data.token, data.org_id)
          navigate('/login?setup_2fa=1')
        } else if (data.org_id) {
          setSession(data.token, data.org_id)
          window.location.href = '/'
        }
      }
    } catch {
      setError('Failed to connect to server')
    } finally {
      setLoading(false)
    }
  }

  if (pending) {
    return (
      <div className="flex min-h-screen items-center justify-center bg-gradient-to-br from-fibratus-900 to-fibratus-600 py-12">
        <div className="w-full max-w-md">
          <div className="text-center mb-8">
            <h1 className="text-3xl font-bold text-white">
              <span className="text-fibratus-300">Fibratus</span> Fleet
            </h1>
          </div>
          <div className="rounded-xl border border-gray-200 dark:border-slate-700 bg-white dark:bg-slate-800 p-8 shadow-sm dark:shadow-slate-900/50 text-center">
            <h2 className="text-xl font-semibold text-gray-900 dark:text-slate-100">Signup received</h2>
            <p className="mt-3 text-sm text-gray-600 dark:text-slate-400">
              Your account request is pending administrator approval. You'll be able to sign in once a root
              admin approves it. No action is needed on your end.
            </p>
            <div className="mt-6">
              <Link to="/login" className="text-fibratus-600 hover:underline text-sm">Back to login</Link>
            </div>
          </div>
        </div>
      </div>
    )
  }

  const update = (field: string) => (e: React.ChangeEvent<HTMLInputElement>) => {
    setForm(prev => ({ ...prev, [field]: e.target.value }))
  }

  return (
    <div className="flex min-h-screen items-center justify-center bg-gradient-to-br from-fibratus-900 to-fibratus-600 py-12">
      <div className="w-full max-w-md">
        <div className="text-center mb-8">
          <h1 className="text-3xl font-bold text-white">
            <span className="text-fibratus-300">Fibratus</span> Fleet
          </h1>
          <p className="mt-2 text-sm text-fibratus-200">Create your fleet management account</p>
        </div>

        <form onSubmit={handleSubmit} className="rounded-xl border border-gray-200 dark:border-slate-700 bg-white dark:bg-slate-800 p-8 shadow-sm dark:shadow-slate-900/50">
          {error && (
            <div className="mb-4 rounded-lg bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800/50 px-4 py-3 text-sm text-red-700 dark:text-red-400">
              {error}
            </div>
          )}

          <div className="space-y-4">
            <div>
              <label className="block text-sm font-medium text-gray-700 dark:text-slate-300">Account Name</label>
              <input
                type="text"
                value={form.account_name}
                onChange={update('account_name')}
                required
                className="mt-1 w-full rounded-lg border border-gray-300 dark:border-slate-600 bg-white dark:bg-slate-700 px-4 py-2.5 text-sm text-gray-900 dark:text-slate-100 placeholder-gray-400 dark:placeholder-slate-500 focus:border-fibratus-500 focus:outline-none focus:ring-1 focus:ring-fibratus-500"
                placeholder="Acme Corp"
              />
              <p className="mt-1 text-xs text-gray-400 dark:text-slate-500">Your company or team name</p>
            </div>
            <div>
              <label className="block text-sm font-medium text-gray-700 dark:text-slate-300">First Organization</label>
              <input
                type="text"
                value={form.org_name}
                onChange={update('org_name')}
                required
                className="mt-1 w-full rounded-lg border border-gray-300 dark:border-slate-600 bg-white dark:bg-slate-700 px-4 py-2.5 text-sm text-gray-900 dark:text-slate-100 placeholder-gray-400 dark:placeholder-slate-500 focus:border-fibratus-500 focus:outline-none focus:ring-1 focus:ring-fibratus-500"
                placeholder="Production"
              />
              <p className="mt-1 text-xs text-gray-400 dark:text-slate-500">e.g., Production, Staging, US-East</p>
            </div>
            <hr className="my-2 dark:border-slate-700" />
            <div>
              <label className="block text-sm font-medium text-gray-700 dark:text-slate-300">Your Name</label>
              <input
                type="text"
                value={form.name}
                onChange={update('name')}
                required
                className="mt-1 w-full rounded-lg border border-gray-300 dark:border-slate-600 bg-white dark:bg-slate-700 px-4 py-2.5 text-sm text-gray-900 dark:text-slate-100 placeholder-gray-400 dark:placeholder-slate-500 focus:border-fibratus-500 focus:outline-none focus:ring-1 focus:ring-fibratus-500"
                placeholder="Jane Smith"
              />
            </div>
            <div>
              <label className="block text-sm font-medium text-gray-700 dark:text-slate-300">Email</label>
              <input
                type="email"
                value={form.email}
                onChange={update('email')}
                required
                className="mt-1 w-full rounded-lg border border-gray-300 dark:border-slate-600 bg-white dark:bg-slate-700 px-4 py-2.5 text-sm text-gray-900 dark:text-slate-100 placeholder-gray-400 dark:placeholder-slate-500 focus:border-fibratus-500 focus:outline-none focus:ring-1 focus:ring-fibratus-500"
                placeholder="jane@acme.com"
              />
            </div>
            <div>
              <label className="block text-sm font-medium text-gray-700 dark:text-slate-300">Password</label>
              <input
                type="password"
                value={form.password}
                onChange={update('password')}
                required
                minLength={8}
                className="mt-1 w-full rounded-lg border border-gray-300 dark:border-slate-600 bg-white dark:bg-slate-700 px-4 py-2.5 text-sm text-gray-900 dark:text-slate-100 focus:border-fibratus-500 focus:outline-none focus:ring-1 focus:ring-fibratus-500"
              />
              <p className="mt-1 text-xs text-gray-400 dark:text-slate-500">Minimum 8 characters</p>
            </div>
          </div>

          <button
            type="submit"
            disabled={loading}
            className="mt-6 w-full rounded-lg bg-fibratus-600 px-4 py-2.5 text-sm font-medium text-white hover:bg-fibratus-700 disabled:opacity-50"
          >
            {loading ? 'Creating account...' : 'Create Account'}
          </button>

          <p className="mt-4 text-center text-sm text-gray-500 dark:text-slate-400">
            Already have an account?{' '}
            <Link to="/login" className="font-medium text-fibratus-600 dark:text-fibratus-400 hover:text-fibratus-500">
              Sign in
            </Link>
          </p>
        </form>
      </div>
    </div>
  )
}
