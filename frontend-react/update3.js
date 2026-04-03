const fs = require('fs');
const file = 'C:/Users/konst/GolandProjects/toDoNotificator/frontend-react/src/components/PomodoroWidget.tsx';
let txt = fs.readFileSync(file, 'utf8');
txt = txt.replace('await completePomodoro(activeSession.id, \\'abandoned\\');\r\n        }\r\n        onTaskUpdated();', 'await completePomodoro(activeSession.id, \\'abandoned\\');\r\n        }\r\n\r\n        if (stopAction === \\'completed\\' && markTaskComplete && activeSession.task_id) {\r\n           await updateTask(activeSession.task_id, { status: \\'done\\' });\r\n        } else if (stopAction === \\'abandoned\\' && activeSession.task_id) {\r\n           await updateTask(activeSession.task_id, { status: \\'burnt\\' });\r\n        }\r\n\r\n        onTaskUpdated();');
fs.writeFileSync(file, txt, 'utf8');
