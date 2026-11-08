import { Check, ChevronDown, Search, X } from 'lucide-react';
import clsx from 'clsx';
import type { SortKey, StatusFilter } from './filters';

const chips: { value: StatusFilter; label: string }[] = [
  { value: 'all', label: 'All' },
  { value: 'running', label: 'Running' },
  { value: 'stopped', label: 'Stopped' },
  { value: 'problems', label: 'Problems' },
];

interface ServerFiltersProps {
  query: string;
  onQueryChange: (value: string) => void;
  status: StatusFilter;
  onStatusChange: (value: StatusFilter) => void;
  sort: SortKey;
  onSortChange: (value: SortKey) => void;
  counts: Record<StatusFilter, number>;
  showReorderHint: boolean;
}

export const ServerFilters = ({
  query,
  onQueryChange,
  status,
  onStatusChange,
  sort,
  onSortChange,
  counts,
  showReorderHint,
}: ServerFiltersProps) => (
  <div className="mb-6">
    <div className="flex flex-wrap items-center gap-3">
      <div className="relative w-full sm:w-72">
        <Search size={16} className="absolute left-3 top-1/2 -translate-y-1/2 text-gray-500" aria-hidden />
        <input
          type="text"
          value={query}
          onChange={(e) => onQueryChange(e.target.value)}
          placeholder="Search servers"
          aria-label="Search servers"
          className="w-full bg-[#1a1a1a] border border-[#3a3a3a] rounded pl-9 pr-8 py-2 text-sm text-white focus:outline-none focus:border-[#E5B80B] focus:ring-1 focus:ring-[#E5B80B]"
        />
        {query && (
          <button
            type="button"
            onClick={() => onQueryChange('')}
            aria-label="Clear search"
            className="absolute right-2 top-1/2 -translate-y-1/2 text-gray-500 hover:text-white transition-colors"
          >
            <X size={14} />
          </button>
        )}
      </div>

      <div role="group" aria-label="Filter by status" className="flex flex-wrap items-center gap-2">
        {chips.map((chip) => {
          const active = status === chip.value;
          return (
            <button
              key={chip.value}
              type="button"
              aria-pressed={active}
              onClick={() => onStatusChange(chip.value)}
              className={clsx(
                'flex items-center gap-1.5 px-3 py-1.5 rounded-full border text-xs font-medium transition-colors',
                active
                  ? 'border-[#E5B80B] bg-[#E5B80B]/10 text-[#E5B80B]'
                  : 'border-[#3a3a3a] text-gray-400 hover:border-gray-500 hover:text-gray-200'
              )}
            >
              {active && <Check size={12} aria-hidden />}
              {chip.label}
              <span className="font-mono text-[11px] opacity-80">{counts[chip.value]}</span>
            </button>
          );
        })}
      </div>

      <div className="relative sm:ml-auto">
        <select
          value={sort}
          onChange={(e) => onSortChange(e.target.value as SortKey)}
          aria-label="Sort servers"
          className="bg-[#1a1a1a] border border-[#3a3a3a] rounded pl-3 pr-8 py-2 text-sm text-white appearance-none cursor-pointer focus:outline-none focus:border-[#E5B80B] focus:ring-1 focus:ring-[#E5B80B]"
        >
          <option value="custom">Custom order</option>
          <option value="name">Name</option>
          <option value="status">Status</option>
          <option value="type">Type</option>
        </select>
        <ChevronDown size={14} className="absolute right-3 top-1/2 -translate-y-1/2 text-gray-500 pointer-events-none" aria-hidden />
      </div>
    </div>
    {showReorderHint && (
      <p className="mt-2 text-xs text-gray-400">Clear the search, filter and sorting to drag servers into a new order.</p>
    )}
  </div>
);
