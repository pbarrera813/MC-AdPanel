import React from 'react';
import { motion } from 'motion/react';
import clsx from 'clsx';
import { useSortable } from '@dnd-kit/sortable';
import { CSS } from '@dnd-kit/utilities';

interface SortableServerCardProps {
  id: string;
  canDrag: boolean;
  className: string;
  onClick: () => void;
  onDoubleClick: () => void;
  onContextMenu: (e: React.MouseEvent<HTMLDivElement>) => void;
  onNodeRef?: (node: HTMLDivElement | null) => void;
  children: React.ReactNode;
}

export const SortableServerCard = ({
  id,
  canDrag,
  className,
  onClick,
  onDoubleClick,
  onContextMenu,
  onNodeRef,
  children,
}: SortableServerCardProps) => {
  const { attributes, listeners, setNodeRef, transform, transition, isDragging } = useSortable({
    id,
    disabled: !canDrag,
  });

  const combinedRef = (node: HTMLDivElement | null) => {
    setNodeRef(node);
    onNodeRef?.(node);
  };

  const style: React.CSSProperties = {
    transform: CSS.Transform.toString(transform),
    transition: transition ?? 'transform 140ms ease-out',
    touchAction: 'pan-y',
    willChange: 'transform',
  };

  return (
    <motion.div
      ref={combinedRef}
      {...(canDrag ? attributes : {})}
      {...(canDrag ? listeners : {})}
      initial={{ opacity: 0, y: 10 }}
      animate={{ opacity: 1, y: 0 }}
      transition={{ type: 'tween', duration: 0.14, ease: 'easeOut' }}
      className={clsx(className, isDragging && 'opacity-30')}
      style={style}
      onClick={onClick}
      onDoubleClick={onDoubleClick}
      onContextMenu={onContextMenu}
    >
      {children}
    </motion.div>
  );
};
