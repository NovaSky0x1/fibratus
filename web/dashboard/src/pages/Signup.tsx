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
      const data = res.data as AuthResponse
      if (data.token && data.org_id) {
        setSession(data.token, data.org_id)
        navigate('/')
      }
    } catch {
      setError('Failed to connect to server')
    } finally {
      setLoading(false)
    }
  }

  const update = (field: string) => (e: React.ChangeEvent<HTMLInputElement>) => {
    setForm(prev => ({ ...prev, [field]: e.target.value }))
  }

  return (
    <div className="flex min-h-screen items-center justify-center bg-gray-50 py-12">
      <div className="w-full max-w-md">
        <div className="text-center mb-8">
          <h1 className="text-3xl font-bold text-gray-900">
            <span className="text-fibratus-600">Fibratus</span> Fleet
          </h1>
          <p className="mt-2 text-sm text-gray-500">Create your fleet management account</p>
        </div>

        <form onSubmit={handleSubmit} className="rounded-xl border border-gray-200 bg-white p-8 shadow-sm">
          {error && (
            <div className="mb-4 rounded-lg bg-red-50 border border-red-200 px-4 py-3 text-sm text-red-700">
              {error}
            </div>
          )}

          <div className="space-y-4">
            <div>
              <label className="block text-sm font-medium text-gray-700">Account Name</label>
              <input
                type="text"
                value={form.account_name}
                onChange={update('account_name')}
                required
                className="mt-1 w-full rounded-lg border border-gray-300 px-4 py-2.5 text-sm focus:border-fibratus-500 focus:outline-none focus:ring-1 focus:ring-fibratus-500"
                placeholder="Acme Corp"
              />
              <p className="mt-1 text-xs text-gray-400">Your company or team name</p>
            </div>
            <div>
              <label className="block text-sm font-medium text-gray-700">First Organization</label>
              <input
                type="text"
                value={form.org_name}
                onChange={update('org_name')}
                required
                className="mt-1 w-full rounded-lg border border-gray-300 px-4 py-2.5 text-sm focus:border-fibratus-500 focus:outline-none focus:ring-1 focus:ring-fibratus-500"
                placeholder="Production"
              />
              <p className="mt-1 text-xs text-gray-400">e.g., Production, Staging, US-East</p>
            </div>
            <hr className="my-2" />
            <div>
              <label className="block text-sm font-medium text-gray-700">Your Name</label>
              <input
                type="text"
                value={form.name}
                onChange={update('name')}
                required
                className="mt-1 w-full rounded-lg border border-gray-300 px-4 py-2.5 text-sm focus:border-fibratus-500 focus:outline-none focus:ring-1 focus:ring-fibratus-500"
                placeholder="Jane Smith"
              />
            </div>
            <div>
              <label className="block text-sm font-medium text-gray-700">Email</label>
              <input
                type="email"
                value={form.email}
                onChange={update('email')}
                required
                className="mt-1 w-full rounded-lg border border-gray-300 px-4 py-2.5 text-sm focus:border-fibratus-500 focus:outline-none focus:ring-1 focus:ring-fibratus-500"
                placeholder="jane@acme.com"
              />
            </div>
            <div>
              <label className="block text-sm font-medium text-gray-700">Password</label>
              <input
                type="password"
                value={form.password}
                onChange={update('password')}
                required
                minLength={8}
                className="mt-1 w-full rounded-lg border border-gray-300 px-4 py-2.5 text-sm focus:border-fibratus-500 focus:outline-none focus:ring-1 focus:ring-fibratus-500"
              />
              <p className="mt-1 text-xs text-gray-400">Minimum 8 characters</p>
            </div>
          </div>

          <button
            type="submit"
            disabled={loading}
            className="mt-6 w-full rounded-lg bg-fibratus-600 px-4 py-2.5 text-sm font-medium text-white hover:bg-fibratus-700 disabled:opacity-50"
          >
            {loading ? 'Creating account...' : 'Create Account'}
          </button>

          <p className="mt-4 text-center text-sm text-gray-500">
            Already have an account?{' '}
            <Link to="/login" className="font-medium text-fibratus-600 hover:text-fibratus-500">
              Sign in
            </Link>
          </p>
        </form>
      </div>
    </div>
  )
}
