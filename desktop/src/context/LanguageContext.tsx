import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useState,
  type ReactNode,
} from 'react';
import {
  createTranslator,
  normalizeLanguage,
  persistLanguage,
  resolveInitialLanguage,
  type Language,
  type Translator,
} from '../i18n';

interface LanguageContextValue {
  language: Language;
  setLanguage: (lang: Language) => void;
  toggleLanguage: () => void;
  t: Translator;
}

const LanguageContext = createContext<LanguageContextValue | null>(null);

export function LanguageProvider({ children }: { children: ReactNode }) {
  const [language, setLanguageState] = useState<Language>(resolveInitialLanguage);
  const t = useMemo(() => createTranslator(language), [language]);

  const setLanguage = useCallback((next: Language) => {
    const normalized = normalizeLanguage(next);
    setLanguageState(normalized);
    persistLanguage(normalized);
  }, []);

  const toggleLanguage = useCallback(() => {
    setLanguage(language === 'zh' ? 'en' : 'zh');
  }, [language, setLanguage]);

  useEffect(() => {
    document.documentElement.lang = language === 'zh' ? 'zh-CN' : 'en';
    document.title = t('app.title');
  }, [language, t]);

  return (
    <LanguageContext.Provider value={{ language, setLanguage, toggleLanguage, t }}>
      {children}
    </LanguageContext.Provider>
  );
}

export function useLanguageContext(): LanguageContextValue {
  const ctx = useContext(LanguageContext);
  if (!ctx) {
    throw new Error('useLanguageContext must be used inside LanguageProvider');
  }
  return ctx;
}
