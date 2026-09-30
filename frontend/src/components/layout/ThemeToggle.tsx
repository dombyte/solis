import { useTheme } from '../theme-provider';
import { Button } from '../ui/button';
import { LineAwesomeIcon } from '../ui/LineAwesomeIcon';

export function ThemeToggle() {
  const { theme, setTheme } = useTheme();

  // "system" follows the OS: toggle away from what is actually shown (like the "d"
  // shortcut), otherwise the first click can pick the theme already on screen.
  const resolvedTheme =
    theme === 'system'
      ? (window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light')
      : theme;

  const toggleTheme = () => {
    setTheme(resolvedTheme === 'light' ? 'dark' : 'light');
  };

  return (
    <Button variant="ghost" size="icon" onClick={toggleTheme} className="h-8 w-8">
      {resolvedTheme === 'light' ? (
        <LineAwesomeIcon icon="la-moon" size="lg" />
      ) : (
        <LineAwesomeIcon icon="la-sun" size="lg" />
      )}
      <span className="sr-only">Toggle theme</span>
    </Button>
  );
}
