const fs = require('fs');
const iconv = require('iconv-lite');

function fixFile(path) {
  const buf = fs.readFileSync(path);
  const decoded = iconv.decode(buf, 'win1251');
  const corrected = decoded.replace(/\?\? Pomodoro/, '?? Pomodoro');
  fs.writeFileSync(path, corrected, 'utf8');
}

fixFile('src/App.tsx');
fixFile('src/components/Login.tsx');
fixFile('src/components/PomodoroWidget.tsx');
console.log('Fixed!');
