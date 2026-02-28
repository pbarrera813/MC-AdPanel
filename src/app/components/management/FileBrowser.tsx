import React, { useState, useEffect, useCallback, useRef } from 'react';
import { Server, FileEntry } from '../../context/ServerContext';
import { Folder, ChevronRight, FileText, Upload, Plus, Trash2, Home, Loader2, ArrowLeft, Download, CheckSquare, Square, Check, Pencil, Search, Maximize2, Minimize2 } from 'lucide-react';
import { motion, AnimatePresence } from 'motion/react';
import clsx from 'clsx';
import { toast } from 'sonner';
import { useEscapeKey } from '../../hooks/useEscapeKey';

interface FileBrowserProps {
  server: Server;
}

interface UploadItem {
  file: File;
  relativePath: string;
}

type UploadConflictAction = 'prompt' | 'replace' | 'skip';

interface UploadConflictState {
  name: string;
}

interface UploadConflictPayload {
  error?: string;
  name?: string;
  path?: string;
}

class UploadConflictError extends Error {
  fileName: string;

  constructor(fileName: string) {
    super(`The destination already has a file named "${fileName}".`);
    this.name = 'UploadConflictError';
    this.fileName = fileName;
  }
}

interface FileSystemEntryLike {
  isFile: boolean;
  isDirectory: boolean;
  name: string;
  fullPath?: string;
}

interface FileSystemFileEntryLike extends FileSystemEntryLike {
  isFile: true;
  file: (callback: (file: File) => void, errorCallback?: (err: unknown) => void) => void;
}

interface FileSystemDirectoryEntryLike extends FileSystemEntryLike {
  isDirectory: true;
  createReader: () => {
    readEntries: (
      successCallback: (entries: FileSystemEntryLike[]) => void,
      errorCallback?: (err: unknown) => void
    ) => void;
  };
}

interface DataTransferItemWithEntry extends DataTransferItem {
  webkitGetAsEntry?: () => FileSystemEntryLike | null;
}

type EditorLanguage = 'json' | 'yaml' | 'toml' | 'properties' | 'ini' | 'xml' | 'log' | 'plain';

const getEditorLanguage = (filePath: string): EditorLanguage => {
  const lower = filePath.toLowerCase();
  if (lower.endsWith('.json')) return 'json';
  if (lower.endsWith('.yml') || lower.endsWith('.yaml')) return 'yaml';
  if (lower.endsWith('.toml')) return 'toml';
  if (lower.endsWith('.properties') || lower.endsWith('.conf') || lower.endsWith('.cfg')) return 'properties';
  if (lower.endsWith('.ini')) return 'ini';
  if (lower.endsWith('.xml')) return 'xml';
  if (lower.endsWith('.log')) return 'log';
  return 'plain';
};

const formatFileTimestamp = (value: string) => {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return '-';

  const hours24 = date.getHours();
  const minutes = String(date.getMinutes()).padStart(2, '0');
  const suffix = hours24 >= 12 ? 'pm' : 'am';
  const hours12 = hours24 % 12 || 12;
  const day = String(date.getDate()).padStart(2, '0');
  const month = String(date.getMonth() + 1).padStart(2, '0');
  const year = date.getFullYear();

  return `${hours12}:${minutes}${suffix} ${day}/${month}/${year}`;
};

const renderValueToken = (value: string) => {
  const trimmed = value.trim();
  if (/^".*"$/.test(trimmed) || /^'.*'$/.test(trimmed)) {
    return <span className="text-amber-300">{value}</span>;
  }
  if (/^(true|false|null)$/i.test(trimmed)) {
    return <span className="text-sky-300">{value}</span>;
  }
  if (/^-?\d+(\.\d+)?$/.test(trimmed)) {
    return <span className="text-[#E5B80B]">{value}</span>;
  }
  return <span className="text-gray-300">{value}</span>;
};

const renderSyntaxLine = (line: string, language: EditorLanguage): React.ReactNode => {
  const lineNoTrim = line;
  const trimmed = lineNoTrim.trim();

  if (trimmed === '') return '\u00a0';

  if (language === 'log') {
    if (lineNoTrim.includes('ERROR') || lineNoTrim.includes('FATAL')) return <span className="text-red-400">{lineNoTrim}</span>;
    if (lineNoTrim.includes('WARN')) return <span className="text-yellow-400">{lineNoTrim}</span>;
    if (lineNoTrim.includes('INFO')) return <span className="text-sky-300">{lineNoTrim}</span>;
    return <span className="text-gray-300">{lineNoTrim}</span>;
  }

  if (language === 'xml') {
    if (trimmed.startsWith('<!--') || trimmed.endsWith('-->')) {
      return <span className="text-green-400/85">{lineNoTrim}</span>;
    }
    const xmlMatch = lineNoTrim.match(/^(\s*)(<\/?)([\w:-]+)(.*?)(\/?>)\s*$/);
    if (xmlMatch) {
      const [, indent, opener, tagName, attrs, closer] = xmlMatch;
      return (
        <>
          <span>{indent}</span>
          <span className="text-sky-300">{opener}</span>
          <span className="text-cyan-300">{tagName}</span>
          <span className="text-amber-300">{attrs}</span>
          <span className="text-sky-300">{closer}</span>
        </>
      );
    }
    return <span className="text-gray-300">{lineNoTrim}</span>;
  }

  if (trimmed.startsWith('#') || trimmed.startsWith(';')) {
    return <span className="text-green-400/85">{lineNoTrim}</span>;
  }

  if (language === 'json') {
    const jsonLine = lineNoTrim.match(/^(\s*)"([^"]+)"(\s*:\s*)(.*?)(,?\s*)$/);
    if (jsonLine) {
      const [, indent, key, sep, value, trailing] = jsonLine;
      return (
        <>
          <span>{indent}</span>
          <span className="text-sky-300">"{key}"</span>
          <span className="text-gray-500">{sep}</span>
          {renderValueToken(value)}
          <span className="text-gray-500">{trailing}</span>
        </>
      );
    }
    return <span className="text-gray-300">{lineNoTrim}</span>;
  }

  if (language === 'yaml' || language === 'toml' || language === 'properties' || language === 'ini') {
    const kvLine = lineNoTrim.match(/^(\s*)([^:=\s][^:=]*?)(\s*[:=]\s*)(.*)$/);
    if (kvLine) {
      const [, indent, key, sep, value] = kvLine;
      return (
        <>
          <span>{indent}</span>
          <span className="text-sky-300">{key}</span>
          <span className="text-gray-500">{sep}</span>
          {renderValueToken(value)}
        </>
      );
    }
    return <span className="text-gray-300">{lineNoTrim}</span>;
  }

  return <span className="text-gray-300">{lineNoTrim}</span>;
};

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
  const [uploadProgress, setUploadProgress] = useState(0);
  const [uploadTotalItems, setUploadTotalItems] = useState(0);
  const [uploadConflict, setUploadConflict] = useState<UploadConflictState | null>(null);
  const [saving, setSaving] = useState(false);
  const [isEditorMaximized, setIsEditorMaximized] = useState(false);
  const [editorSearch, setEditorSearch] = useState('');
  const [activeSearchMatch, setActiveSearchMatch] = useState(0);
  const [folderSearch, setFolderSearch] = useState('');
  const [downloading, setDownloading] = useState(false);
  const [newlyUploaded, setNewlyUploaded] = useState<Set<string>>(new Set());
  const [isRenameModalOpen, setIsRenameModalOpen] = useState(false);
  const [renameTarget, setRenameTarget] = useState('');
  const [newName, setNewName] = useState('');
  const [renaming, setRenaming] = useState(false);
  const fileInputRef = useRef<HTMLInputElement>(null);
  const folderInputRef = useRef<HTMLInputElement>(null);
  const uploadAbortRef = useRef<AbortController | null>(null);
  const uploadConflictResolverRef = useRef<((action: Exclude<UploadConflictAction, 'prompt'>) => void) | null>(null);
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
  useEscapeKey(!!uploadConflict, () => {
    if (uploadConflictResolverRef.current) {
      uploadConflictResolverRef.current('skip');
      uploadConflictResolverRef.current = null;
    }
    setUploadConflict(null);
  });
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
  const normalizedFolderSearch = folderSearch.trim().toLowerCase();
  const filteredEntries = React.useMemo(() => {
    if (!normalizedFolderSearch) return entries;
    return entries.filter((entry) => entry.name.toLowerCase().includes(normalizedFolderSearch));
  }, [entries, normalizedFolderSearch]);
  const allVisibleSelected = filteredEntries.length > 0 && filteredEntries.every((entry) => selectedNames.has(entry.name));

  const highlightFolderMatch = (value: string) => {
    if (!normalizedFolderSearch) return value;
    const lowerValue = value.toLowerCase();
    const nodes: React.ReactNode[] = [];
    let cursor = 0;
    let key = 0;
    while (cursor < value.length) {
      const matchIndex = lowerValue.indexOf(normalizedFolderSearch, cursor);
      if (matchIndex === -1) {
        nodes.push(<span key={`tail-${key}`}>{value.slice(cursor)}</span>);
        break;
      }
      if (matchIndex > cursor) {
        nodes.push(<span key={`text-${key}`}>{value.slice(cursor, matchIndex)}</span>);
        key += 1;
      }
      const matchEnd = matchIndex + normalizedFolderSearch.length;
      nodes.push(
        <mark key={`match-${key}`} className="bg-[#E5B80B]/65 text-black rounded px-0.5">
          {value.slice(matchIndex, matchEnd)}
        </mark>
      );
      key += 1;
      cursor = matchEnd;
    }
    return nodes;
  };

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
    if (allVisibleSelected) {
      setSelectedNames(prev => {
        const next = new Set(prev);
        filteredEntries.forEach((entry) => next.delete(entry.name));
        return next;
      });
    } else {
      setSelectedNames(prev => {
        const next = new Set(prev);
        filteredEntries.forEach((entry) => next.add(entry.name));
        return next;
      });
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
      setIsEditorMaximized(false);
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

  useEffect(() => {
    setSelectedNames(new Set());
  }, [folderSearch]);

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
  const editorLanguage = React.useMemo(
    () => (editingFile ? getEditorLanguage(editingFile.path) : 'plain'),
    [editingFile]
  );
  const highlightedContent = React.useMemo(() => {
    const term = editorSearch.trim();
    if (!term || searchMatches.length === 0) {
      const lines = (editContent || '').split('\n');
      if (lines.length === 0) return ' ';
      return lines.map((line, index) => (
        <React.Fragment key={`line-${index}`}>
          {renderSyntaxLine(line, editorLanguage)}
          {index < lines.length - 1 ? '\n' : null}
        </React.Fragment>
      ));
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
  }, [editContent, editorSearch, searchMatches, activeSearchMatch, editorLanguage]);

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

  const uploadSingleFile = (
    item: UploadItem,
    signal: AbortSignal,
    onProgress: (loaded: number, total: number) => void,
    conflictAction: UploadConflictAction = 'prompt'
  ): Promise<'uploaded' | 'replaced' | 'skipped'> =>
    new Promise((resolve, reject) => {
      const xhr = new XMLHttpRequest();
      xhr.open('POST', `/api/servers/${server.id}/files/upload?path=${encodeURIComponent(currentPath)}`);
      xhr.upload.onprogress = (event) => {
        if (event.lengthComputable) {
          onProgress(event.loaded, event.total);
        }
      };

      const onAbort = () => {
        xhr.abort();
      };
      signal.addEventListener('abort', onAbort, { once: true });

      xhr.onload = () => {
        signal.removeEventListener('abort', onAbort);
        if (xhr.status === 409) {
          try {
            const payload = JSON.parse(xhr.responseText) as UploadConflictPayload;
            if (payload?.error === 'file_exists') {
              reject(new UploadConflictError(payload.name || item.file.name));
              return;
            }
          } catch {
            // ignore parse errors and fall through to generic error
          }
        }
        if (xhr.status >= 200 && xhr.status < 300) {
          try {
            const payload = JSON.parse(xhr.responseText) as { status?: string };
            if (payload?.status === 'skipped') {
              resolve('skipped');
              return;
            }
            if (payload?.status === 'replaced') {
              resolve('replaced');
              return;
            }
          } catch {
            // ignore parse errors and treat as uploaded
          }
          resolve('uploaded');
          return;
        }
        reject(new Error(`Failed to upload ${item.relativePath || item.file.name}`));
      };

      xhr.onerror = () => {
        signal.removeEventListener('abort', onAbort);
        reject(new Error(`Failed to upload ${item.relativePath || item.file.name}`));
      };

      xhr.onabort = () => {
        signal.removeEventListener('abort', onAbort);
        reject(new DOMException('Upload cancelled', 'AbortError'));
      };

      const formData = new FormData();
      formData.append('file', item.file);
      formData.append('relativePath', item.relativePath);
      formData.append('conflictAction', conflictAction);
      xhr.send(formData);
    });

  const getTargetPath = (item: UploadItem) =>
    currentPath === '.' ? item.relativePath : `${currentPath}/${item.relativePath}`;

  const checkPathExists = async (path: string): Promise<boolean> => {
    const res = await fetch(`/api/servers/${server.id}/files/exists?path=${encodeURIComponent(path)}`);
    if (!res.ok) {
      throw new Error('Failed to check existing files');
    }
    const data = await res.json() as { exists?: boolean };
    return data.exists === true;
  };

  const requestConflictAction = (fileName: string) =>
    new Promise<Exclude<UploadConflictAction, 'prompt'>>((resolve) => {
      uploadConflictResolverRef.current = resolve;
      setUploadConflict({ name: fileName });
    });

  const handleUploadItems = async (items: UploadItem[]) => {
    if (items.length === 0) return;

    const controller = new AbortController();
    uploadAbortRef.current = controller;
    setIsUploading(true);
    setUploadProgress(0);
    setUploadTotalItems(items.length);

    const totalBytes = items.reduce((sum, item) => sum + item.file.size, 0) || items.length;
    let uploadedBytes = 0;
    let uploadedCount = 0;
    let skippedCount = 0;

    try {
      for (const item of items) {
        if (controller.signal.aborted) {
          throw new DOMException('Upload cancelled', 'AbortError');
        }

        let action: UploadConflictAction = 'prompt';
        const targetPath = getTargetPath(item);
        const exists = await checkPathExists(targetPath);
        if (exists) {
          action = await requestConflictAction(item.relativePath || item.file.name);
          uploadConflictResolverRef.current = null;
          setUploadConflict(null);
          if (action === 'skip') {
            skippedCount += 1;
            uploadedBytes += item.file.size || 1;
            const skippedProgress = Math.min(100, Math.round((uploadedBytes / totalBytes) * 100));
            setUploadProgress(skippedProgress);
            continue;
          }
        }

        let skippedByConflict = false;
        let result: 'uploaded' | 'replaced' | 'skipped' = 'uploaded';
        for (;;) {
          try {
            result = await uploadSingleFile(item, controller.signal, (loaded, total) => {
              const completedBefore = uploadedBytes;
              const totalForFile = total > 0 ? total : item.file.size || 1;
              const loadedForFile = Math.min(loaded, totalForFile);
              const progress = Math.min(100, Math.round(((completedBefore + loadedForFile) / totalBytes) * 100));
              setUploadProgress(progress);
            }, action);
            break;
          } catch (err) {
            if (err instanceof UploadConflictError) {
              const choice = await requestConflictAction(err.fileName);
              uploadConflictResolverRef.current = null;
              setUploadConflict(null);
              if (choice === 'skip') {
                skippedByConflict = true;
                break;
              }
              action = 'replace';
              continue;
            }
            throw err;
          }
        }
        if (skippedByConflict) {
          skippedCount += 1;
          uploadedBytes += item.file.size || 1;
          const skippedProgress = Math.min(100, Math.round((uploadedBytes / totalBytes) * 100));
          setUploadProgress(skippedProgress);
          continue;
        }
        if (result === 'skipped') {
          skippedCount += 1;
        } else {
          uploadedCount += 1;
        }
        uploadedBytes += item.file.size || 1;
        const progress = Math.min(100, Math.round((uploadedBytes / totalBytes) * 100));
        setUploadProgress(progress);
      }

      const uploadedNames = new Set(items.map(item => item.relativePath.split('/')[0] || item.file.name));
      setNewlyUploaded(uploadedNames);
      if (uploadedCount > 0 && skippedCount > 0) {
        toast.success(`Uploaded ${uploadedCount} item(s), skipped ${skippedCount}`);
      } else if (uploadedCount > 0) {
        toast.success(uploadedCount === 1 ? 'File uploaded successfully' : 'Files uploaded successfully');
      } else {
        toast.info(skippedCount === 1 ? 'File skipped' : `Skipped ${skippedCount} item(s)`);
      }
      setIsUploadModalOpen(false);
      fetchFiles();
    } catch (err) {
      if (err instanceof DOMException && err.name === 'AbortError') {
        toast.info('Upload cancelled');
      } else {
        toast.error(err instanceof Error ? err.message : 'Upload failed');
      }
    } finally {
      if (uploadConflictResolverRef.current) {
        uploadConflictResolverRef.current('skip');
        uploadConflictResolverRef.current = null;
      }
      setUploadConflict(null);
      uploadAbortRef.current = null;
      setIsUploading(false);
      setUploadProgress(0);
      setUploadTotalItems(0);
    }
  };

  const normalizeRelativePath = (path: string) => path.replace(/\\/g, '/').replace(/^\/+/, '');

  const toUploadItemsFromFileList = (fileList: FileList | null): UploadItem[] => {
    if (!fileList || fileList.length === 0) return [];
    return Array.from(fileList).map((file) => {
      const relativePath = normalizeRelativePath((file as File & { webkitRelativePath?: string }).webkitRelativePath || file.name);
      return {
        file,
        relativePath,
      };
    });
  };

  const readDroppedEntries = async (
    entry: FileSystemEntryLike,
    prefix: string,
    output: UploadItem[]
  ): Promise<void> => {
    if (entry.isFile) {
      const fileEntry = entry as FileSystemFileEntryLike;
      const file = await new Promise<File>((resolve, reject) => {
        fileEntry.file(resolve, reject);
      });
      output.push({
        file,
        relativePath: normalizeRelativePath(`${prefix}${file.name}`),
      });
      return;
    }

    if (!entry.isDirectory) return;

    const dirEntry = entry as FileSystemDirectoryEntryLike;
    const reader = dirEntry.createReader();
    let entries: FileSystemEntryLike[] = [];
    do {
      entries = await new Promise<FileSystemEntryLike[]>((resolve, reject) => {
        reader.readEntries(resolve, reject);
      });
      for (const child of entries) {
        await readDroppedEntries(child, `${prefix}${entry.name}/`, output);
      }
    } while (entries.length > 0);
  };

  const extractUploadItemsFromDataTransfer = async (dataTransfer: DataTransfer): Promise<UploadItem[]> => {
    const items: UploadItem[] = [];
    const dtItems = dataTransfer.items ? Array.from(dataTransfer.items) : [];

    const getEntry = (item: DataTransferItem): FileSystemEntryLike | null => {
      try {
        const entryGetter = (item as DataTransferItemWithEntry).webkitGetAsEntry;
        if (typeof entryGetter !== 'function') return null;
        return entryGetter.call(item);
      } catch {
        return null;
      }
    };

    const hasEntries = dtItems.some((item) => !!getEntry(item));

    if (hasEntries) {
      for (const item of dtItems) {
        const entry = getEntry(item);
        if (!entry) continue;
        try {
          await readDroppedEntries(entry, '', items);
        } catch (err) {
          console.error('Failed to parse dropped entry tree:', err);
        }
      }
      return items;
    }

    return toUploadItemsFromFileList(dataTransfer.files);
  };

  const handleFileUpload = async (fileList: FileList | null) => {
    const items = toUploadItemsFromFileList(fileList);
    await handleUploadItems(items);
  };

  const handleDropUpload = async (e: React.DragEvent<HTMLDivElement>) => {
    e.preventDefault();
    try {
      const items = await extractUploadItemsFromDataTransfer(e.dataTransfer);
      if (items.length === 0) {
        toast.error('No uploadable files were found in the dropped content');
        return;
      }
      await handleUploadItems(items);
    } catch (err) {
      console.error('Drop upload failed:', err);
      toast.error('Failed to process dropped folders/files');
    }
  };

  const handleCancelUpload = () => {
    if (uploadConflictResolverRef.current) {
      uploadConflictResolverRef.current('skip');
      uploadConflictResolverRef.current = null;
      setUploadConflict(null);
      return;
    }
    if (uploadAbortRef.current) {
      uploadAbortRef.current.abort();
      return;
    }
    setIsUploadModalOpen(false);
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
    <div className="h-full min-h-0 overflow-hidden flex flex-col bg-[#1e1e1d] p-3 md:p-6 relative">
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
          <AnimatePresence initial={false}>
            {hasSelection && (
              <motion.div
                key="selection-actions"
                initial={{ opacity: 0, y: -6, scale: 0.98 }}
                animate={{ opacity: 1, y: 0, scale: 1 }}
                exit={{ opacity: 0, y: -6, scale: 0.98 }}
                transition={{ duration: 0.18, ease: 'easeOut' }}
                className="flex items-center gap-2"
              >
              <motion.button
                initial={{ opacity: 0, scale: 0.9 }}
                animate={{ opacity: 1, scale: 1 }}
                onClick={toggleSelectAll}
                className="flex items-center gap-1 px-2 py-1.5 text-xs text-gray-400 hover:text-white bg-[#252524] border border-[#3a3a3a] rounded transition-colors"
                title={allVisibleSelected ? 'Uncheck All' : 'Check All'}
              >
                {allVisibleSelected ? <CheckSquare size={14} /> : <Square size={14} />}
                {allVisibleSelected ? 'Uncheck All' : 'Check All'}
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
              </motion.div>
            )}
          </AnimatePresence>
          {currentPath !== '.' && (
            <button
              onClick={navigateUp}
              className="flex items-center gap-1 px-3 py-1.5 bg-[#252524] border border-[#3a3a3a] text-sm text-gray-300 rounded hover:bg-[#333] transition-colors"
            >
              <ArrowLeft size={16} /> Back
            </button>
          )}
          <div className="relative w-44 md:w-52">
            <Search size={14} className="absolute left-2 top-1/2 -translate-y-1/2 text-gray-500" />
            <input
              type="text"
              value={folderSearch}
              onChange={(e) => setFolderSearch(e.target.value)}
              placeholder="Search in this folder"
              className="w-full bg-[#1a1a1a] border border-[#3a3a3a] rounded pl-7 pr-2 py-1.5 text-xs text-white focus:outline-none focus:border-[#E5B80B]"
            />
          </div>
          <span className="text-[11px] text-gray-500 w-16 text-right">{filteredEntries.length} found</span>
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
        ) : filteredEntries.length === 0 ? (
          <div className="flex items-center justify-center py-20 text-gray-500 text-sm">
            {normalizedFolderSearch ? 'No matches in this folder' : 'Empty directory'}
          </div>
        ) : (
          filteredEntries.map((entry) => {
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
                  {highlightFolderMatch(entry.name)}
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
                <span className="text-xs text-gray-600 font-mono whitespace-nowrap">{formatFileTimestamp(entry.modTime)}</span>
                <span className="text-xs text-gray-600 font-mono whitespace-nowrap">{entry.size}</span>
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
            className={clsx(
              "absolute inset-0 z-40 bg-[#1e1e1d]/95 backdrop-blur flex items-stretch justify-stretch p-0",
              !isEditorMaximized && "md:items-center md:justify-center md:p-8"
            )}
          >
            <div
              className={clsx(
                "w-full h-full bg-[#252524] border border-[#404040] rounded-none shadow-2xl flex flex-col",
                !isEditorMaximized && "md:max-w-4xl md:h-3/4 md:rounded-lg"
              )}
            >
              <div className="px-3 md:px-4 py-3 border-b border-[#404040] bg-[#2a2a29]">
                <div className="flex flex-col gap-3 md:flex-row md:items-center md:justify-between">
                  <span className="font-mono text-white flex items-center gap-2 truncate min-w-0">
                  <FileText size={16} /> {editingFile.path}
                  </span>
                  <div className="flex flex-wrap items-center gap-2 md:justify-end">
                    <div className="flex flex-wrap items-center gap-2 w-full sm:w-auto">
                      <div className="relative flex-1 min-w-0 sm:min-w-[180px]">
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
                      <span className="text-[11px] text-gray-500 text-left sm:text-right w-full sm:w-auto">{searchMatches.length} found</span>
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
                        onClick={() => setIsEditorMaximized(prev => !prev)}
                        className="px-3 py-1 text-xs bg-[#333] text-gray-300 rounded hover:bg-[#444] flex items-center gap-1"
                        title={isEditorMaximized ? 'Restore' : 'Maximize'}
                      >
                        {isEditorMaximized ? <Minimize2 size={12} /> : <Maximize2 size={12} />}
                        {isEditorMaximized ? 'Restore' : 'Maximize'}
                      </button>
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
                      spellCheck={false}
                      autoCorrect="off"
                      autoCapitalize="off"
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
              className="w-full max-w-lg bg-[#252524] border border-[#404040] rounded-lg shadow-2xl p-6 relative"
              onDragOver={(e) => e.preventDefault()}
              onDrop={handleDropUpload}
            >
              <h3 className="text-xl font-bold text-white mb-4">Upload Files</h3>
              {isUploading ? (
                <div className="h-48 flex flex-col justify-center gap-4">
                  <div className="flex items-center justify-between text-sm">
                    <p className="text-gray-300">
                      {uploadTotalItems <= 1 ? 'Uploading file...' : 'Uploading files...'}
                    </p>
                    <span className="text-[#E5B80B] font-bold">{uploadProgress}%</span>
                  </div>
                  <div className="w-full h-2 bg-[#1a1a1a] border border-[#3a3a3a] rounded overflow-hidden">
                    <div
                      className="h-full bg-[#E5B80B] transition-[width] duration-200 ease-out"
                      style={{ width: `${uploadProgress}%` }}
                    />
                  </div>
                  <p className="text-xs text-gray-500">
                    {uploadTotalItems <= 1 ? 'Please wait while the upload finishes.' : `Processing ${uploadTotalItems} items...`}
                  </p>
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
                  <input
                    ref={folderInputRef}
                    type="file"
                    multiple
                    className="hidden"
                    {...({ webkitdirectory: '', directory: '' } as any)}
                    onChange={(e) => handleFileUpload(e.target.files)}
                  />
                  <Upload size={32} className="mb-2" />
                  <p>Drag & drop files or folders here</p>
                  <p className="text-xs text-gray-600 mt-2">click to choose files</p>
                  <button
                    type="button"
                    onClick={(e) => {
                      e.stopPropagation();
                      folderInputRef.current?.click();
                    }}
                    className="mt-3 px-3 py-1.5 text-xs border border-[#3a3a3a] rounded bg-[#1a1a1a] text-gray-300 hover:border-[#E5B80B] hover:text-[#E5B80B] transition-colors"
                  >
                    Choose Folder
                  </button>
                </div>
              )}
              <div className="flex justify-end gap-3">
                <button
                  onClick={handleCancelUpload}
                  className="px-4 py-2 bg-[#333] hover:bg-[#404040] text-gray-200 rounded font-medium"
                >
                  {isUploading ? 'Cancel Upload' : 'Cancel'}
                </button>
              </div>

              <AnimatePresence>
                {uploadConflict && (
                  <motion.div
                    initial={{ opacity: 0 }}
                    animate={{ opacity: 1 }}
                    exit={{ opacity: 0 }}
                    className="absolute inset-0 z-10 bg-black/55 backdrop-blur-[1px] rounded-lg flex items-center justify-center p-4"
                  >
                    <motion.div
                      initial={{ y: 8, scale: 0.98 }}
                      animate={{ y: 0, scale: 1 }}
                      exit={{ y: 8, scale: 0.98 }}
                      className="w-full max-w-md bg-[#202020] border border-[#404040] rounded-lg p-4"
                    >
                      <p className="text-sm text-gray-200">
                        The destination already has a file named "{uploadConflict.name}".
                      </p>
                      <div className="mt-4 flex justify-end gap-2">
                        <button
                          onClick={() => {
                            uploadConflictResolverRef.current?.('skip');
                            uploadConflictResolverRef.current = null;
                            setUploadConflict(null);
                          }}
                          className="px-3 py-1.5 bg-[#333] hover:bg-[#404040] text-gray-200 rounded text-sm"
                        >
                          Skip
                        </button>
                        <button
                          onClick={() => {
                            uploadConflictResolverRef.current?.('replace');
                            uploadConflictResolverRef.current = null;
                            setUploadConflict(null);
                          }}
                          className="px-3 py-1.5 bg-[#E5B80B] hover:bg-[#d4a90a] text-black rounded text-sm font-bold"
                        >
                          Replace
                        </button>
                      </div>
                    </motion.div>
                  </motion.div>
                )}
              </AnimatePresence>
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
