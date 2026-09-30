import { CalendarIcon } from "lucide-react";

import { INSERT_DATETIME_COMMAND } from "@/components/editor/extensions/date-time-extension";
import { ComponentPickerOption } from "@/components/editor/plugins/picker/component-picker-option";

export function DateTimePickerPlugin({ title }: { title: string }) {
  return new ComponentPickerOption(title, {
    icon: <CalendarIcon className="size-4" />,
    keywords: ["date", "calendar", "time", "today"],
    onSelect: (_, editor) => {
      const dateTime = new Date();
      dateTime.setHours(0, 0, 0, 0);
      editor.dispatchCommand(INSERT_DATETIME_COMMAND, { dateTime });
    },
  });
}
