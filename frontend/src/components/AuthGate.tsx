import { type FormEvent, type ReactNode, useEffect, useState } from 'react';
import { createOperatorSession, fetchOperatorSession } from '../api/client';

interface AuthGateProps {
  children: ReactNode;
}

type AuthState = 'checking' | 'authenticated' | 'anonymous' | 'unauthenticated';

export function AuthGate({ children }: AuthGateProps) {
  const [state, setState] = useState<AuthState>('checking');
  const [operator, setOperator] = useState('operator');
  const [token, setToken] = useState('');
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);

  useEffect(() => {
    let cancelled = false;
    fetchOperatorSession()
      .then((session) => {
        if (cancelled) return;
        setState(session.authenticated ? 'authenticated' : 'unauthenticated');
      })
      .catch(() => {
        if (cancelled) return;
        setState('unauthenticated');
      });

    return () => {
      cancelled = true;
    };
  }, []);

  async function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setSubmitting(true);
    setError(null);
    try {
      const session = await createOperatorSession(token, operator);
      if (!session.authenticated) {
        setError('Token non valido');
        return;
      }
      setToken('');
      setState('authenticated');
    } catch {
      setError('Token non valido');
    } finally {
      setSubmitting(false);
    }
  }

  if (state === 'authenticated' || state === 'anonymous') {
    return <>{children}</>;
  }

  if (state === 'checking') {
    return (
      <div className="flex h-screen w-screen items-center justify-center bg-bg text-on-bg">
        <div className="text-sm text-on-bg-secondary">Loading</div>
      </div>
    );
  }

  return (
    <div className="flex h-screen w-screen items-center justify-center bg-bg px-6 text-on-bg">
      <form
        className="w-full max-w-sm rounded-lg border border-outline-subtle bg-surface p-6 shadow-xl"
        onSubmit={handleSubmit}
      >
        <div className="mb-6">
          <h1 className="text-xl font-semibold">Theia</h1>
          <p className="mt-1 text-sm text-on-bg-secondary">Accesso operatore</p>
        </div>
        <label className="mb-4 block">
          <span className="mb-2 block text-sm font-medium">Operatore</span>
          <input
            className="w-full rounded-md border border-outline-subtle bg-bg px-3 py-2 text-sm outline-none focus:border-primary"
            autoComplete="username"
            value={operator}
            onChange={(event) => setOperator(event.target.value)}
          />
        </label>
        <label className="mb-5 block">
          <span className="mb-2 block text-sm font-medium">Token</span>
          <input
            className="w-full rounded-md border border-outline-subtle bg-bg px-3 py-2 text-sm outline-none focus:border-primary"
            autoComplete="current-password"
            type="password"
            value={token}
            onChange={(event) => setToken(event.target.value)}
          />
        </label>
        {error && <div className="mb-4 text-sm text-warning">{error}</div>}
        <button
          className="w-full rounded-md bg-primary px-4 py-2 text-sm font-semibold text-on-primary disabled:cursor-not-allowed disabled:opacity-60"
          disabled={submitting || token.trim() === ''}
          type="submit"
        >
          {submitting ? 'Accesso in corso' : 'Entra'}
        </button>
      </form>
    </div>
  );
}
