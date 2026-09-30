import { useTheme } from '../theme-provider';
import { Button } from '../ui/button';
import { LineAwesomeIcon } from '../ui/LineAwesomeIcon';

export function ThemeToggle() {
  // Toggle away from what is actually shown (like the "d" shortcut), so under "system"
  // the first click never picks the theme already on screen.
  const { resolvedTheme, setTheme } = useTheme();

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
