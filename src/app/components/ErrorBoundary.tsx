import React from 'react';
import { Button } from './ui/button';

type State = { error: Error | null; info: React.ErrorInfo | null };

export default class ErrorBoundary extends React.Component<{ children: React.ReactNode }, State> {
  constructor(props: any) {
    super(props);
    this.state = { error: null, info: null };
  }

  static getDerivedStateFromError(error: Error) {
    return { error, info: null } as State;
  }

  componentDidCatch(error: Error, info: React.ErrorInfo) {
    // Log to console so devtools / terminal will show it
    // eslint-disable-next-line no-console
    console.error('Uncaught error in component tree:', error, info);
    this.setState({ error, info });
  }

  reset = () => this.setState({ error: null, info: null });

  render() {
    if (this.state.error) {
      return (
        <div className="h-screen flex items-center justify-center bg-[#1e1e1d] p-6">
          <div className="max-w-3xl w-full bg-[#252524] border border-[#3a3a3a] rounded-lg p-6 text-sm text-gray-200">
            <h2 className="text-xl font-bold mb-2">Something went wrong</h2>
            <p className="text-gray-400 mb-4">The application encountered an error while rendering this page. The error has been logged to the console.</p>
            <div className="mb-4 bg-[#0f0f0f] border border-[#404040] rounded p-3 overflow-auto max-h-48 text-xs font-mono text-red-300">
              <div className="font-medium text-sm text-red-400">{this.state.error?.message}</div>
              <pre className="whitespace-pre-wrap mt-2 text-xs text-gray-300">{this.state.info?.componentStack || this.state.error?.stack}</pre>
            </div>
            <div className="flex gap-3 justify-end">
              <button onClick={() => window.location.reload()} className="px-4 py-2 bg-[#333] hover:bg-[#404040] rounded text-gray-200">Reload</button>
              <button onClick={this.reset} className="px-4 py-2 bg-[#E5B80B] text-black rounded font-bold">Dismiss</button>
            </div>
          </div>
        </div>
      );
    }

    return this.props.children as any;
  }
}
