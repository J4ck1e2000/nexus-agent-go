import { useLanguage } from '../../hooks/useLanguage';

/** Segmented EN / 中文 pill switch, mirroring the web language toggle. */
export default function LanguageToggle() {
  const { language, setLanguage } = useLanguage();

  const optionClass = (active: boolean): string =>
    `rounded-full px-3 py-1 text-xs font-medium transition cursor-pointer ${
      active ? 'bg-ink-strong text-[#f8f4ed]' : 'text-muted hover:text-ink'
    }`;

  return (
    <div className="flex items-center gap-0.5 rounded-full border border-line bg-panel-soft p-1" role="group" aria-label="Language">
      <button type="button" className={optionClass(language === 'en')} onClick={() => setLanguage('en')} aria-pressed={language === 'en'}>
        EN
      </button>
      <button type="button" className={optionClass(language === 'zh')} onClick={() => setLanguage('zh')} aria-pressed={language === 'zh'}>
        中文
      </button>
    </div>
  );
}
