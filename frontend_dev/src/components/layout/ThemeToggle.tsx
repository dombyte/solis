import { useTheme } from '../theme-provider';
import { Button } from '../ui/button';
import { LineAwesomeIcon } from '../ui/LineAwesomeIcon';

export function ThemeToggle() {
  const { theme, setTheme } = useTheme();

  const toggleTheme = () => {
    setTheme(theme === 'light' ? 'dark' : 'light');
  };

  return (
    <Button variant="ghost" size="icon" onClick={toggleTheme} className="h-8 w-8">
      {theme === 'light' ? (
        <LineAwesomeIcon icon="la-moon" size="lg" />
      ) : (
        <LineAwesomeIcon icon="la-sun" size="lg" />
      )}
      <span className="sr-only">Toggle theme</span>
    </Button>
  );
}
