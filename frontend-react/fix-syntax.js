const fs = require('fs');
const file = 'C:/Users/konst/GolandProjects/toDoNotificator/frontend-react/src/components/PomodoroWidget.tsx';
let txt = fs.readFileSync(file, 'utf8');
txt = txt.replace(/<div key=\{i\} className=\{w\-3 h\-3 rounded\-full \} title=\"\u041f\u0440\u043e\u0433\u0440\u0435\u0441\u0441\"><\/div>/g, '<div key={i} className={w-3 h-3 rounded-full } title=\"\u041f\u0440\u043e\u0433\u0440\u0435\u0441\u0441\"></div>');

txt = txt.replace(/<div className=\{w\-3 h\-3 rounded\-full animate\-pulse \} \/>/g, '<div className={w-3 h-3 rounded-full animate-pulse } />');

txt = txt.replace(/<span className=\{ ext\-sm font\-semibold uppercase tracking\-wider \}>/g, '<span className={	ext-sm font-semibold uppercase tracking-wider }>');

txt = txt.replace(/<div key=\{i\} className=\{w\-2 h\-2 rounded\-full \} title=\"\u041f\u0440\u043e\u0433\u0440\u0435\u0441\u0441 \u0434\u043e \u0434\u043b\u0438\u043d\u043d\u043e\u0433\u043e \u043f\u0435\u0440\u0435\u0440\u044b\u0432\u0430\"\/>/g, '<div key={i} className={w-2 h-2 rounded-full } title=\"\u041f\u0440\u043e\u0433\u0440\u0435\u0441\u0441 \u0434\u043e \u0434\u043b\u0438\u043d\u043d\u043e\u0433\u043e \u043f\u0435\u0440\u0435\u0440\u044b\u0432\u0430\"/>');

txt = txt.replace(/<button onClick=\{confirmStop\} className=\{px\-8 py\-3 rounded\-xl font\-bold text\-white shadow\-sm \}>\u0414\u0430<\/button>/g, '<button onClick={confirmStop} className={px-8 py-3 rounded-xl font-bold text-white shadow-sm }>\u0414\u0430</button>');

fs.writeFileSync(file, txt, 'utf8');
