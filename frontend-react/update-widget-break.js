const fs = require('fs');
let content = fs.readFileSync('C:\\Users\\konst\\GolandProjects\\toDoNotificator\\frontend-react\\src\\components\\PomodoroWidget.tsx', 'utf8');

content = content.replace(
  /const handleTakeCoffeeBreak = async \(\) => \{\s*try \{\s*const \{ session \} = await pausePomodoro\(activeSession!\.id\);\s*takeBreak\(session\);\s*setMode\('break'\);\s*addToast\('Время отдохнуть 5 минут ☕', 'info'\);\s*\} catch \(e: any\) \{/,
  \const handleTakeCoffeeBreak = async () => {
    try {
      await pausePomodoro(activeSession!.id);
      takeBreak({ ...activeSession!, breaks_used: 1 }); // Manually update session object
      setMode('break');
      addToast('Время отдохнуть 5 минут ☕', 'info');
    } catch (e: any) {\
);

fs.writeFileSync('C:\\Users\\konst\\GolandProjects\\toDoNotificator\\frontend-react\\src\\components\\PomodoroWidget.tsx', content, 'utf8');
