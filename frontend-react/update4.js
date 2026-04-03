const fs = require('fs');
const file = 'C:/Users/konst/GolandProjects/toDoNotificator/frontend-react/src/components/PomodoroWidget.tsx';
let txt = fs.readFileSync(file, 'utf8');
txt = txt.replace(/import \{ type Task, updateTask \} from '\.\.\/api\/tasks';/g, 'import { type Task } from \\'../api/tasks\\';');
txt = txt.replace(/import \{ Play, CheckCircle \} from 'lucide-react';/g, 'import { Play } from \\'lucide-react\\';');
txt = txt.replace(/  const \[markTaskComplete, setMarkTaskComplete\] = useState\(true\);\r?\n\r?\n  const focusedTask = activeSession\?\.task_id \? tasks\.find\(t => t\.id === activeSession\.task_id\) : null;\r?\n/g, '');
fs.writeFileSync(file, txt, 'utf8');
