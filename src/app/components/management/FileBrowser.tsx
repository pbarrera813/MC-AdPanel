import React, { useState, useEffect, useCallback, useRef } from 'react';
import { Server, FileEntry } from '../../context/ServerContext';
import { Folder, ChevronRight, FileText, Upload, Plus, Trash2, Home, Loader2, ArrowLeft, Download, CheckSquare, Square, Check, Pencil, Search } from 'lucide-react';
import { motion, AnimatePresence } from 'motion/react';
import clsx from 'clsx';
import { toast } from 'sonner';
import { useEscapeKey } from '../../hooks/useEscapeKey';

interface FileBrowserProps {
  server: Server;
}

export const FileBrowser = ({ server }: FileBrowserProps) => {
  const [currentPath, setCurrentPath] = useState('.');
  const [entries, setEntries] = useState<FileEntry[]>([]);
  const [loading, setLoading] = useState(true);
  const [selectedNames, setSelectedNames] = useState<Set<string>>(new Set());

  // Modals state
  const [editingFile, setEditingFile] = useState<{ path: string; content: string } | null>(null);
  const [editContent, setEditContent] = useState('');
  const [isUploadModalOpen, setIsUploadModalOpen] = useState(false);
  const [isNewFolderModalOpen, setIsNewFolderModalOpen] = useState(false);
  const [newFolderName, setNewFolderName] = useState('New Folder');
  const [isUploading, setIsUploading] = useState(false);
  const [saving, setSaving] = useState(false);
  const [editorSearch, setEditorSearch] = useState('');
  const [activeSearchMatch, setActiveSearchMatch] = useState(0);
  const [downloading, setDownloading] = useState(false);
  const [newlyUploaded, setNewlyUploaded] = useState<Set<string>>(new Set());
  const [isRenameModalOpen, setIsRenameModalOpen] = useState(false);
  const [renameTarget, setRenameTarget] = useState('');
  const [newName, setNewName] = useState('');
  const [renaming, setRenaming] = useState(false);
  const fileInputRef = useRef<HTMLInputElement>(null);
  const editorTextareaRef = useRef<HTMLTextAreaElement>(null);
  const lineNumbersRef = useRef<HTMLDivElement>(null);
  const highlightRef = useRef<HTMLPreElement>(null);

  const hasSelection = selectedNames.size > 0;

  const fetchFiles = useCallback(async () => {
    setLoading(true);
    try {
      const res = await fetch(`/api/servers/${server.id}/files?path=${encodeURIComponent(currentPath)}`);
      if (!res.ok) throw new Error('Failed to fetch files');
      const data: FileEntry[] = await res.json();
      setEntries(data || []);
    } catch (err) {
      toast.error('Failed to load directory');
      setEntries([]);
    } finally {
      setLoading(false);
    }
  }, [server.id, currentPath]);

  useEffect(() => {
    fetchFiles();
    setSelectedNames(new Set());
    setNewlyUploaded(new Set());
  }, [fetchFiles]);

  useEscapeKey(!!editingFile, () => setEditingFile(null));
  useEscapeKey(isUploadModalOpen, () => setIsUploadModalOpen(false));
  useEscapeKey(isNewFolderModalOpen, () => setIsNewFolderModalOpen(false));
  useEscapeKey(isRenameModalOpen, () => setIsRenameModalOpen(false));

  const navigateTo = (name: string) => {
    const newPath = currentPath === '.' ? name : `${currentPath}/${name}`;
    setCurrentPath(newPath);
  };

  const navigateUp = () => {
    if (currentPath === '.') return;
    const parts = currentPath.split('/');
    parts.pop();
    setCurrentPath(parts.length === 0 ? '.' : parts.join('/'));
  };

  const navigateToRoot = () => {
    setCurrentPath('.');
  };

  const breadcrumbs = currentPath === '.' ? [] : currentPath.split('/');

  const toggleSelection = (name: string) => {
    setSelectedNames(prev => {
      const next = new Set(prev);
      if (next.has(name)) {
        next.delete(name);
      } else {
        next.add(name);
      }
      return next;
    });
  };

  const toggleSelectAll = () => {
    if (selectedNames.size === entries.length) {
      setSelectedNames(new Set());
    } else {
      setSelectedNames(new Set(entries.map(e => e.name)));
    }
  };

  const handleFileOpen = async (name: string) => {
    const filePath = currentPath === '.' ? name : `${currentPath}/${name}`;
    try {
      const res = await fetch(`/api/servers/${server.id}/files/content?path=${encodeURIComponent(filePath)}`);
      if (!res.ok) throw new Error('Failed to read file');
      const content = await res.text();
      setEditingFile({ path: filePath, content });
      setEditContent(content);
      setEditorSearch('');
      setActiveSearchMatch(0);
    } catch (err) {
      toast.error('Failed to open file');
    }
  };

  const escapeRegExp = (value: string) => value.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');

  const searchMatches = React.useMemo(() => {
    const term = editorSearch.trim();
    if (!term) return [] as number[];
    const re = new RegExp(escapeRegExp(term), 'gi');
    const indices: number[] = [];
    let match: RegExpExecArray | null;
    while ((match = re.exec(editContent)) !== null) {
      indices.push(match.index);
      if (match[0].length === 0) re.lastIndex += 1;
    }
    return indices;
  }, [editContent, editorSearch]);

  useEffect(() => {
    setActiveSearchMatch(0);
  }, [editorSearch, editContent]);

  const jumpToSearchMatch = (direction: 'next' | 'prev') => {
    const term = editorSearch.trim();
    const textarea = editorTextareaRef.current;
    if (!term || !textarea || searchMatches.length === 0) return;
    const nextMatch = direction === 'next'
      ? (activeSearchMatch + 1) % searchMatches.length
      : (activeSearchMatch - 1 + searchMatches.length) % searchMatches.length;
    const targetIndex = searchMatches[nextMatch];
    setActiveSearchMatch(nextMatch);

    textarea.focus();
    textarea.setSelectionRange(targetIndex, targetIndex + term.length);
  };

  const lineCount = React.useMemo(() => Math.max(1, editContent.split('\n').length), [editContent]);
  const highlightedContent = React.useMemo(() => {
    const term = editorSearch.trim();
    if (!term || searchMatches.length === 0) {
      return editContent || ' ';
    }

    const parts: React.ReactNode[] = [];
    let lastIndex = 0;
    for (let i = 0; i < searchMatches.length; i += 1) {
      const start = searchMatches[i];
      const end = start + term.length;
      if (start > lastIndex) {
        parts.push(
          <span key={`text-${start}`}>{editContent.slice(lastIndex, start)}</span>
        );
      }
      parts.push(
        <span
          key={`match-${start}`}
          className={clsx(
            'rounded px-0.5',
            i === activeSearchMatch ? 'bg-[#E5B80B] text-black' : 'bg-[#E5B80B]/50 text-[#111]'
          )}
        >
          {editContent.slice(start, end)}
        </span>
      );
      lastIndex = end;
    }
    if (lastIndex < editContent.length) {
      parts.push(<span key={`text-tail`}>{editContent.slice(lastIndex)}</span>);
    }
    return parts;
  }, [editContent, editorSearch, searchMatches, activeSearchMatch]);

  const handleSaveFile = async () => {
    if (!editingFile) return;
    setSaving(true);
    try {
      const res = await fetch(`/api/servers/${server.id}/files/content`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ path: editingFile.path, content: editContent }),
      });
      if (!res.ok) throw new Error('Failed to save file');
      toast.success('File saved');
      setEditingFile(null);
    } catch (err) {
      toast.error('Failed to save file');
    } finally {
      setSaving(false);
    }
  };

  const handleDelete = async () => {
    if (selectedNames.size === 0) return;
    const paths = Array.from(selectedNames).map(name =>
      currentPath === '.' ? name : `${currentPath}/${name}`
    );
    try {
      for (const targetPath of paths) {
        const res = await fetch(`/api/servers/${server.id}/files?path=${encodeURIComponent(targetPath)}`, {
          method: 'DELETE',
        });
        if (!res.ok) throw new Error('Failed to delete');
      }
      toast.success(`Deleted ${paths.length} item(s)`);
      setSelectedNames(new Set());
      fetchFiles();
    } catch (err) {
      toast.error('Failed to delete');
    }
  };

  const handleDownload = async () => {
    if (selectedNames.size === 0) return;
    setDownloading(true);

    const paths = Array.from(selectedNames).map(name =>
      currentPath === '.' ? name : `${currentPath}/${name}`
    );

    try {
      const res = await fetch(`/api/servers/${server.id}/files/download`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ paths }),
      });
      if (!res.ok) throw new Error('Failed to download');

      const blob = await res.blob();
      const url = URL.createObjectURL(blob);
      const a = document.createElement('a');
      a.href = url;

      // Get filename from Content-Disposition header or use defaults
      const disposition = res.headers.get('Content-Disposition');
      let filename = paths.length === 1 ? paths[0].split('/').pop() || 'download' : 'batch.zip';
      if (disposition) {
        const match = disposition.match(/filename="?([^"]+)"?/);
        if (match) filename = match[1];
      }

      a.download = filename;
      document.body.appendChild(a);
      a.click();
      document.body.removeChild(a);
      URL.revokeObjectURL(url);
      toast.success('Download started');
    } catch (err) {
      toast.error('Failed to download');
    } finally {
      setDownloading(false);
    }
  };

  const handleNewFolder = async () => {
    const folderPath = currentPath === '.' ? newFolderName : `${currentPath}/${newFolderName}`;
    try {
      const res = await fetch(`/api/servers/${server.id}/files/mkdir`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ path: folderPath }),
      });
      if (!res.ok) throw new Error('Failed to create folder');
      toast.success('Folder created');
      setIsNewFolderModalOpen(false);
      setNewFolderName('New Folder');
      fetchFiles();
    } catch (err) {
      toast.error('Failed to create folder');
    }
  };

  const handleFileUpload = async (fileList: FileList | null) => {
    if (!fileList || fileList.length === 0) return;
    setIsUploading(true);
    try {
      for (const file of Array.from(fileList)) {
        const formData = new FormData();
        formData.append('file', file);
        const res = await fetch(`/api/servers/${server.id}/files/upload?path=${encodeURIComponent(currentPath)}`, {
          method: 'POST',
          body: formData,
        });
        if (!res.ok) throw new Error(`Failed to upload ${file.name}`);
      }
      const uploadedNames = new Set(Array.from(fileList).map(f => f.name));
      setNewlyUploaded(uploadedNames);
      toast.success('File(s) uploaded successfully');
      setIsUploadModalOpen(false);
      fetchFiles();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Upload failed');
    } finally {
      setIsUploading(false);
    }
  };

  const openRenameModal = (name: string) => {
    setRenameTarget(name);
    setNewName(name);
    setIsRenameModalOpen(true);
  };

  const handleRename = async () => {
    if (!renameTarget || !newName.trim() || newName === renameTarget) {
      setIsRenameModalOpen(false);
      return;
    }
    setRenaming(true);
    const oldPath = currentPath === '.' ? renameTarget : `${currentPath}/${renameTarget}`;
    try {
      const res = await fetch(`/api/servers/${server.id}/files/rename`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ oldPath, newName: newName.trim() }),
      });
      if (!res.ok) {
        const data = await res.json().catch(() => ({}));
        throw new Error(data.error || 'Failed to rename');
      }
      toast.success('Renamed successfully');
      setIsRenameModalOpen(false);
      setSelectedNames(new Set());
      fetchFiles();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to rename');
    } finally {
      setRenaming(false);
    }
  };

  const isTextFile = (name: string) =>
    /\.(txt|properties|json|log|yml|yaml|toml|cfg|conf|ini|xml|csv|md|sh|bat|secret)$/i.test(name);

  const serverDirName = server.name.replace(/\s+/g, '_');

  return (
    <div className="flex-1 overflow-hidden flex flex-col bg-[#1e1e1d] p-3 md:p-6 relative">
      <div className="flex justify-between items-center mb-4">
        {/* Breadcrumb */}
        <div className="flex items-center gap-1 text-sm text-gray-400 font-mono truncate max-w-xl">
          <button onClick={navigateToRoot} className="hover:text-[#E5B80B] transition-colors flex items-center gap-1">
            <Home size={14} />
            /AdPanel/Servers/{serverDirName}
          </button>
          {breadcrumbs.map((part, i) => (
            <React.Fragment key={i}>
              <ChevronRight size={12} className="text-gray-600" />
              <button
                onClick={() => {
                  const newPath = breadcrumbs.slice(0, i + 1).join('/');
                  setCurrentPath(newPath);
                }}
                className="hover:text-[#E5B80B] transition-colors"
              >
                {part}
              </button>
            </React.Fragment>
          ))}
        </div>
        <div className="flex items-center gap-2">
          {hasSelection && (
            <>
              <motion.button
                initial={{ opacity: 0, scale: 0.9 }}
                animate={{ opacity: 1, scale: 1 }}
                onClick={toggleSelectAll}
                className="flex items-center gap-1 px-2 py-1.5 text-xs text-gray-400 hover:text-white bg-[#252524] border border-[#3a3a3a] rounded transition-colors"
                title={selectedNames.size === entries.length ? 'Uncheck All' : 'Check All'}
              >
                {selectedNames.size === entries.length ? <CheckSquare size={14} /> : <Square size={14} />}
                {selectedNames.size === entries.length ? 'Uncheck All' : 'Check All'}
              </motion.button>
              <motion.button
                initial={{ opacity: 0, scale: 0.9 }}
                animate={{ opacity: 1, scale: 1 }}
                onClick={handleDownload}
                disabled={downloading}
                className="p-2 hover:bg-blue-900/30 text-blue-400 rounded transition-colors disabled:opacity-50"
                title={`Download ${selectedNames.size} item(s)`}
              >
                {downloading ? <Loader2 size={18} className="animate-spin" /> : <Download size={18} />}
              </motion.button>
              <motion.button
                initial={{ opacity: 0, scale: 0.9 }}
                animate={{ opacity: 1, scale: 1 }}
                onClick={handleDelete}
                className="p-2 hover:bg-red-900/30 text-red-400 rounded transition-colors"
                title="Delete selected"
              >
                <Trash2 size={18} />
              </motion.button>
            </>
          )}
          {currentPath !== '.' && (
            <button
              onClick={navigateUp}
              className="flex items-center gap-1 px-3 py-1.5 bg-[#252524] border border-[#3a3a3a] text-sm text-gray-300 rounded hover:bg-[#333] transition-colors"
            >
              <ArrowLeft size={16} /> Back
            </button>
          )}
          <button
            onClick={() => setIsNewFolderModalOpen(true)}
            className="flex items-center gap-2 px-3 py-1.5 bg-[#252524] border border-[#3a3a3a] text-sm text-gray-300 rounded hover:bg-[#333] hover:text-white transition-colors"
          >
            <Plus size={16} /> New Folder
          </button>
          <button
            onClick={() => setIsUploadModalOpen(true)}
            className="flex items-center gap-2 px-3 py-1.5 bg-[#E5B80B] text-black border border-[#E5B80B] text-sm rounded font-bold hover:bg-[#d4a90a] transition-colors shadow-lg shadow-[#E5B80B]/10"
          >
            <Upload size={16} /> Upload
          </button>
        </div>
      </div>

      {/* File List */}
      <div
        className="flex-1 bg-[#202020] border border-[#3a3a3a] rounded-lg overflow-y-auto p-2"
        onClick={() => setSelectedNames(new Set())}
      >
        {loading ? (
          <div className="flex items-center justify-center py-20">
            <Loader2 size={24} className="animate-spin text-gray-500" />
          </div>
        ) : entries.length === 0 ? (
          <div className="flex items-center justify-center py-20 text-gray-500 text-sm">
            Empty directory
          </div>
        ) : (
          entries.map((entry) => {
            const isSelected = selectedNames.has(entry.name);
            return (
              <div
                key={entry.name}
                onClick={(e) => { e.stopPropagation(); toggleSelection(entry.name); }}
                onDoubleClick={(e) => {
                  e.stopPropagation();
                  if (entry.type === 'folder') {
                    navigateTo(entry.name);
                  } else if (isTextFile(entry.name)) {
                    handleFileOpen(entry.name);
                  }
                }}
                className={clsx(
                  'flex items-center gap-2 py-1.5 px-3 cursor-pointer text-sm select-none group transition-colors rounded',
                  isSelected
                    ? 'bg-[#3a3a3a] text-white'
                    : 'text-gray-300 hover:bg-[#2a2a29]'
                )}
              >
                {/* Checkbox — visible when any selection exists */}
                {hasSelection && (
                  <span className={clsx('flex-shrink-0', isSelected ? 'text-[#E5B80B]' : 'text-gray-600')}>
                    {isSelected ? <Check size={14} /> : <Square size={14} />}
                  </span>
                )}
                <span className={entry.type === 'folder' ? 'text-[#E5B80B]' : 'text-gray-400'}>
                  {entry.type === 'folder' ? (
                    <Folder size={16} fill="currentColor" className="opacity-20" />
                  ) : (
                    <FileText size={16} />
                  )}
                </span>
                <span className="flex-1 truncate">
                  {entry.name}
                  {newlyUploaded.has(entry.name) && (
                    <span className="ml-2 text-[#E5B80B] text-xs font-bold">New!</span>
                  )}
                </span>
                {isSelected && selectedNames.size === 1 && (
                  <button
                    onClick={(e) => { e.stopPropagation(); openRenameModal(entry.name); }}
                    className="flex-shrink-0 p-1 rounded text-gray-500 hover:text-[#E5B80B] hover:bg-[#2a2a29] transition-colors"
                    title="Rename"
                  >
                    <Pencil size={14} />
                  </button>
                )}
                <span className="text-xs text-gray-600 font-mono">{entry.size}</span>
              </div>
            );
          })
        )}
      </div>

      {/* Editor Modal */}
      <AnimatePresence>
        {editingFile && (
          <motion.div
            initial={{ opacity: 0, scale: 0.95 }}
            animate={{ opacity: 1, scale: 1 }}
            exit={{ opacity: 0, scale: 0.95 }}
            className="absolute inset-0 z-40 bg-[#1e1e1d]/95 backdrop-blur flex items-stretch justify-stretch p-0 md:items-center md:justify-center md:p-8"
          >
            <div className="w-full h-full bg-[#252524] border border-[#404040] rounded-none md:max-w-4xl md:h-3/4 md:rounded-lg shadow-2xl flex flex-col">
              <div className="px-3 md:px-4 py-3 border-b border-[#404040] bg-[#2a2a29]">
                <div className="flex flex-col gap-3 md:flex-row md:items-center md:justify-between">
                  <span className="font-mono text-white flex items-center gap-2 truncate min-w-0">
                  <FileText size={16} /> {editingFile.path}
                  </span>
                  <div className="flex flex-wrap items-center gap-2 md:justify-end">
                    <div className="flex flex-wrap items-center gap-2 w-full sm:w-auto">
                      <div className="relative flex-1 min-w-[180px]">
                        <Search size={14} className="absolute left-2 top-1/2 -translate-y-1/2 text-gray-500" />
                        <input
                          type="text"
                          value={editorSearch}
                          onChange={(e) => setEditorSearch(e.target.value)}
                          onKeyDown={(e) => {
                            if (e.key === 'Enter') {
                              e.preventDefault();
                              if (searchMatches.length > 0) {
                                jumpToSearchMatch(e.shiftKey ? 'prev' : 'next');
                              }
                            }
                          }}
                          placeholder="Find exact word"
                          className="w-full sm:w-56 bg-[#1a1a1a] border border-[#3a3a3a] rounded pl-7 pr-2 py-1 text-xs text-white focus:outline-none focus:border-[#E5B80B]"
                        />
                      </div>
                      <span className="text-[11px] text-gray-500 min-w-[60px] text-right">{searchMatches.length} found</span>
                      <button
                        onClick={() => jumpToSearchMatch('prev')}
                        className="px-2 py-1 text-xs bg-[#333] text-gray-300 rounded hover:bg-[#444]"
                        disabled={searchMatches.length === 0}
                      >
                        Prev
                      </button>
                      <button
                        onClick={() => jumpToSearchMatch('next')}
                        className="px-2 py-1 text-xs bg-[#333] text-gray-300 rounded hover:bg-[#444]"
                        disabled={searchMatches.length === 0}
                      >
                        Next
                      </button>
                    </div>
                    <div className="flex gap-2">
                      <button
                        onClick={handleSaveFile}
                        disabled={saving}
                        className="px-3 py-1 text-xs bg-[#E5B80B] text-black rounded font-bold hover:bg-[#d4a90a] disabled:opacity-50"
                      >
                        {saving ? 'Saving...' : 'Save'}
                      </button>
                      <button
                        onClick={() => setEditingFile(null)}
                        className="px-3 py-1 text-xs bg-[#333] text-gray-300 rounded hover:bg-[#444]"
                      >
                        Close
                      </button>
                    </div>
                  </div>
                </div>
              </div>
              <div className="flex-1 p-2 md:p-4 overflow-hidden">
                <div className="h-full border border-[#3a3a3a] rounded overflow-hidden flex">
                  <div
                    ref={lineNumbersRef}
                    className="w-10 md:w-12 bg-transparent border-r border-[#3a3a3a]/40 text-[#E5B80B]/85 text-xs font-mono leading-6 py-2 px-1 md:px-2 text-right select-none overflow-hidden"
                  >
                    {Array.from({ length: lineCount }, (_, i) => (
                      <div key={i}>{i + 1}</div>
                    ))}
                  </div>
                  <div className="relative w-full h-full overflow-hidden">
                    <pre
                      ref={highlightRef}
                      aria-hidden="true"
                      className="absolute inset-0 p-2 m-0 text-sm leading-6 font-mono whitespace-pre-wrap break-words text-gray-300 pointer-events-none overflow-hidden"
                    >
                      {highlightedContent}
                    </pre>
                    <textarea
                      ref={editorTextareaRef}
                      className="absolute inset-0 w-full h-full bg-transparent text-transparent caret-[#E5B80B] selection:bg-[#E5B80B]/35 font-mono text-sm leading-6 resize-none focus:outline-none p-2 overflow-auto"
                      value={editContent}
                      onChange={(e) => setEditContent(e.target.value)}
                      onKeyDown={(e) => {
                        if (e.key !== 'Tab') return;
                        e.preventDefault();
                        const target = e.currentTarget;
                        const { selectionStart, selectionEnd, value } = target;
                        const indent = '\t';
                        if (selectionStart === selectionEnd) {
                          const next = value.slice(0, selectionStart) + indent + value.slice(selectionEnd);
                          setEditContent(next);
                          requestAnimationFrame(() => {
                            target.selectionStart = selectionStart + indent.length;
                            target.selectionEnd = selectionStart + indent.length;
                          });
                          return;
                        }

                        const selectedText = value.slice(selectionStart, selectionEnd);
                        const lineStart = value.lastIndexOf('\n', selectionStart - 1) + 1;
                        const selectedLines = value.slice(lineStart, selectionEnd);
                        const indentedLines = selectedLines.replace(/^/gm, indent);
                        const next = value.slice(0, lineStart) + indentedLines + value.slice(selectionEnd);
                        setEditContent(next);
                        const added = indentedLines.length - selectedLines.length;
                        requestAnimationFrame(() => {
                          target.selectionStart = selectionStart + indent.length;
                          target.selectionEnd = selectionEnd + added;
                        });
                      }}
                      onScroll={(e) => {
                        if (lineNumbersRef.current) {
                          lineNumbersRef.current.scrollTop = e.currentTarget.scrollTop;
                        }
                        if (highlightRef.current) {
                          highlightRef.current.scrollTop = e.currentTarget.scrollTop;
                          highlightRef.current.scrollLeft = e.currentTarget.scrollLeft;
                        }
                      }}
                    />
                  </div>
                </div>
              </div>
            </div>
          </motion.div>
        )}
      </AnimatePresence>

      {/* Upload Modal */}
      <AnimatePresence>
        {isUploadModalOpen && (
          <div className="absolute inset-0 z-50 flex items-center justify-center bg-black/60 backdrop-blur-sm p-4">
            <motion.div
              initial={{ opacity: 0, scale: 0.95 }}
              animate={{ opacity: 1, scale: 1 }}
              exit={{ opacity: 0, scale: 0.95 }}
              className="w-full max-w-lg bg-[#252524] border border-[#404040] rounded-lg shadow-2xl p-6"
              onDragOver={(e) => e.preventDefault()}
              onDrop={(e) => { e.preventDefault(); handleFileUpload(e.dataTransfer.files); }}
            >
              <h3 className="text-xl font-bold text-white mb-4">Upload Files</h3>
              {isUploading ? (
                <div className="h-48 flex flex-col items-center justify-center text-[#E5B80B]">
                  <Loader2 size={48} className="animate-spin mb-4" />
                  <p className="text-gray-300">Uploading files...</p>
                </div>
              ) : (
                <div
                  className="border-2 border-dashed border-[#404040] rounded-lg h-48 flex flex-col items-center justify-center text-gray-400 mb-6 cursor-pointer hover:border-[#E5B80B] hover:text-[#E5B80B] transition-colors bg-[#202020]"
                  onClick={() => fileInputRef.current?.click()}
                >
                  <input
                    ref={fileInputRef}
                    type="file"
                    multiple
                    className="hidden"
                    onChange={(e) => handleFileUpload(e.target.files)}
                  />
                  <Upload size={32} className="mb-2" />
                  <p>Drag & drop files here</p>
                  <p className="text-xs text-gray-600 mt-2">or click to browse</p>
                </div>
              )}
              <div className="flex justify-end gap-3">
                <button
                  onClick={() => setIsUploadModalOpen(false)}
                  className="px-4 py-2 bg-[#333] hover:bg-[#404040] text-gray-200 rounded font-medium"
                  disabled={isUploading}
                >
                  Cancel
                </button>
              </div>
            </motion.div>
          </div>
        )}
      </AnimatePresence>

      {/* New Folder Modal */}
      <AnimatePresence>
        {isNewFolderModalOpen && (
          <div className="absolute inset-0 z-50 flex items-center justify-center bg-black/60 backdrop-blur-sm p-4">
            <motion.div
              initial={{ opacity: 0, scale: 0.95 }}
              animate={{ opacity: 1, scale: 1 }}
              exit={{ opacity: 0, scale: 0.95 }}
              className="w-full max-w-md bg-[#252524] border border-[#404040] rounded-lg shadow-2xl p-6"
            >
              <h3 className="text-xl font-bold text-white mb-4">Create New Folder</h3>
              <div className="mb-6">
                <label className="block text-sm text-gray-400 mb-2">Folder Name</label>
                <input
                  type="text"
                  value={newFolderName}
                  onChange={(e) => setNewFolderName(e.target.value)}
                  className="w-full bg-[#1a1a1a] border border-[#3a3a3a] rounded p-3 text-white focus:outline-none focus:border-[#E5B80B] focus:ring-1 focus:ring-[#E5B80B]"
                  autoFocus
                  onKeyDown={(e) => e.key === 'Enter' && handleNewFolder()}
                />
              </div>
              <div className="flex justify-end gap-3">
                <button
                  onClick={() => setIsNewFolderModalOpen(false)}
                  className="px-4 py-2 bg-[#333] hover:bg-[#404040] text-gray-200 rounded font-medium"
                >
                  Cancel
                </button>
                <button
                  onClick={handleNewFolder}
                  className="px-4 py-2 bg-[#E5B80B] hover:bg-[#d4a90a] text-black rounded font-bold"
                >
                  Create
                </button>
              </div>
            </motion.div>
          </div>
        )}
      </AnimatePresence>

      {/* Rename Modal */}
      <AnimatePresence>
        {isRenameModalOpen && (
          <div className="absolute inset-0 z-50 flex items-center justify-center bg-black/60 backdrop-blur-sm p-4">
            <motion.div
              initial={{ opacity: 0, scale: 0.95 }}
              animate={{ opacity: 1, scale: 1 }}
              exit={{ opacity: 0, scale: 0.95 }}
              className="w-full max-w-md bg-[#252524] border border-[#404040] rounded-lg shadow-2xl p-6"
            >
              <h3 className="text-xl font-bold text-white mb-4">Rename</h3>
              <div className="mb-6">
                <label className="block text-sm text-gray-400 mb-2">New Name</label>
                <input
                  type="text"
                  value={newName}
                  onChange={(e) => setNewName(e.target.value)}
                  className="w-full bg-[#1a1a1a] border border-[#3a3a3a] rounded p-3 text-white focus:outline-none focus:border-[#E5B80B] focus:ring-1 focus:ring-[#E5B80B]"
                  autoFocus
                  onKeyDown={(e) => e.key === 'Enter' && handleRename()}
                  disabled={renaming}
                />
              </div>
              <div className="flex justify-end gap-3">
                <button
                  onClick={() => setIsRenameModalOpen(false)}
                  className="px-4 py-2 bg-[#333] hover:bg-[#404040] text-gray-200 rounded font-medium"
                  disabled={renaming}
                >
                  Cancel
                </button>
                <button
                  onClick={handleRename}
                  className="px-4 py-2 bg-[#E5B80B] hover:bg-[#d4a90a] text-black rounded font-bold disabled:opacity-50"
                  disabled={renaming || !newName.trim() || newName === renameTarget}
                >
                  {renaming ? 'Renaming...' : 'Rename'}
                </button>
              </div>
            </motion.div>
          </div>
        )}
      </AnimatePresence>
    </div>
  );
};
