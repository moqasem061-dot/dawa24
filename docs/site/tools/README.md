# مولّد شرح الملفات

- `extract.py` بيقرا تعليقات كل ملف ويكتب `hints.tsv`.
- `d1.tsv` إلى `d4.tsv` فيها الوصف العربي المكتوب يدويًا (المسار ثم Tab ثم الوصف). ملفات d2 مساراتها نسبية لـ internal/modules، وd3 لـ internal/ui.
- `build.py` بيجمع الوصف مع القواعد التلقائية (الاختبارات، الملفات المولدة، الـ migrations، الترجمات، الوثائق) ويكتب `../files.js`، وبيطبع أي ملف ملوش وصف.

```bash
python docs/site/tools/extract.py
python docs/site/tools/build.py
```
