import type { ReactNode } from "react";
import { useSortable } from "@dnd-kit/sortable";
import { CSS } from "@dnd-kit/utilities";
import { GripVertical } from "lucide-react";
import { Button } from "@/components/ui/button";
import { TableRow } from "@/components/ui/table";

interface SortableNodeRowProps {
  id: number;
  name: string;
  disabled: boolean;
  children: (handle: ReactNode) => ReactNode;
}

export function SortableNodeRow({ id, name, disabled, children }: SortableNodeRowProps) {
  const { attributes, listeners, setNodeRef, setActivatorNodeRef, transform, transition, isDragging } = useSortable({ id, disabled });

  return (
    <TableRow
      ref={setNodeRef}
      data-node-id={id}
      data-drag-placeholder={isDragging || undefined}
      style={{ transform: CSS.Transform.toString(transform), transition }}
      className={isDragging
        ? "bg-sky-500/10 hover:bg-sky-500/10 [&>td]:shadow-[inset_0_2px_0_#38bdf8,inset_0_-2px_0_#38bdf8] [&>td>*]:invisible"
        : undefined}
    >
      {children(
        <Button
          ref={setActivatorNodeRef}
          type="button"
          variant="ghost"
          size="icon-sm"
          className="touch-none cursor-grab active:cursor-grabbing disabled:cursor-not-allowed"
          disabled={disabled}
          {...attributes}
          {...listeners}
          title="拖动调整顺序；也可按空格后用上下方向键移动，Esc 取消"
          aria-label={`拖动排序：${name}`}
        >
          <GripVertical className="size-4 text-muted-foreground" />
        </Button>,
      )}
    </TableRow>
  );
}
