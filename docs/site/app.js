(function () {
  'use strict';

  var DIRS = {
    '.': 'جذر المشروع: ملفات البناء والنشر والإعدادات العامة ووثائق البداية.',
    '.github/workflows': 'خط التكامل المستمر على GitHub.',
    '.specify': 'أداة Spec Kit لتخطيط الميزات أثناء التطوير. مش جزء من تشغيل المنصة ومش لازم لأي نشر.',
    'cmd/server': 'برنامج سيرفر الويب. هنا بيتجمع كل شيء: الإعدادات، قاعدة البيانات، الموديولات، المسارات، والجسور مع تيليجرام وواتساب والتسويق. أي ربط بين موديولين بيحصل هنا لأن الموديولات ممنوع تستورد بعض.',
    'cmd/worker': 'برنامج المهام الخلفية. نفس الصورة بس بأمر مختلف. بيشغل طوابير River: الاستيراد، الذكاء الاصطناعي، الإشعارات، الفهارس، الصيانة، والطلب الذكي.',
    'cmd/cli': 'أداة التشغيل: تطبيق الـ migrations، البيانات المرجعية، الحسابات التجريبية، وإصلاح عمليات الاستيراد العالقة.',
    'cmd/dbcheck': 'أدوات فحص قاعدة البيانات الحية وأمان الصفوف. بتقرا بس إلا لو طلبت غير كده.',
    'cmd/etl': 'نقل البيانات من قاعدة Laravel القديمة (MariaDB) لـ PostgreSQL.',
    'cmd/loadseed': 'ملء قاعدة بيانات تجريبية ببيانات سوق واقعية لاختبارات الحمل.',
    'cmd/migratecheck': 'تجربة الـ migrations الجديدة على قاعدة حقيقية مع التراجع التلقائي قبل الإنتاج.',
    'db': 'قاعدة البيانات: ملفات الـ migrations مضمنة في البرنامج.',
    'db/migrations': 'كل تغيير في هيكل قاعدة البيانات، مرقم بالترتيب. كل تغيير له ملف up للتطبيق وملف down للتراجع. أي ملف اتنفذ على الإنتاج ممنوع يتعدل؛ التغيير الجديد يبقى ملف جديد برقم أكبر.',
    'docs': 'الوثائق. أهم ما فيها مجلد modules (توثيق كل موديول) وملف adr (قرارات المعمارية). باقي الملفات خطط ومراجعات من مراحل البناء، مفيدة كتاريخ بس مش مصدر الحقيقة الحالي.',
    'docs/modules': 'توثيق كل موديول بالتفصيل، مكتوب مع الكود ومحدث معاه. أول مكان يتقري قبل تعديل موديول.',
    'docs/site': 'موقع التوثيق ده نفسه.',
    'internal/platform': 'البنية التحتية المشتركة: الإعدادات، قاعدة البيانات، الكاش، الصلاحيات، بوابة الذكاء الاصطناعي، HTTP، الطوابير. ممنوع تستورد أي موديول أعمال.',
    'internal/platform/authctx': 'هوية المستخدم الحالي داخل كل طلب، وmiddleware الصلاحيات والاعتماد.',
    'internal/platform/rbac': 'نظام الصلاحيات والأدوار كله: تعريف كل صلاحية، القوائم الجانبية لكل لوحة، والقارئ اللي بيجيب صلاحيات المستخدم الحالية من قاعدة البيانات.',
    'internal/platform/database': 'الاتصال بـ PostgreSQL، الـ transactions المرتبطة بالمنشأة، ومحرك الـ migrations.',
    'internal/platform/gateway': 'البوابة الوحيدة للذكاء الاصطناعي. المنصة ما تعرفش اسم أي مزود أو موديل برا المجلد ده.',
    'internal/platform/config': 'قراءة كل متغيرات البيئة والتحقق منها عند الإقلاع.',
    'internal/platform/httpx': 'طبقة HTTP: الأمان، CSRF، تحديد المعدل، والمهل الزمنية.',
    'internal/platform/antiscrape': 'حماية الكتالوج ودليل الموردين من الكشط الآلي.',
    'internal/platform/pagecontrol': 'تفعيل وتعطيل أي صفحة من لوحة الإدارة بدون نشر جديد.',
    'internal/platform/progress': 'نقل تقدم عمليات الاستيراد لحظيًا للمتصفح.',
    'internal/platform/importrun': 'نموذج موحد لجلسات الاستيراد الدائمة.',
    'internal/platform/importjobs': 'عمال الاستيراد في الطابور.',
    'internal/platform/queue': 'المهام الخلفية بمكتبة River.',
    'internal/platform/aiusage': 'سجل استهلاك الذكاء الاصطناعي.',
    'internal/shared': 'حزم صغيرة مستقلة بدون أي اعتماد داخلي: الفلوس، الترجمة، العربي، الأخطاء، قراءة Excel، ومحرك المطابقة.',
    'internal/shared/productmatch': 'محرك المطابقة الحتمي: بيقرا ملف المورد أو الصيدلية، يفهم الأعمدة، ويطابق كل صف بمنتج في الكتالوج بدون ذكاء اصطناعي. نفس الملف بيطلع نفس النتيجة دايمًا. ده قلب الاستيراد والطلب الذكي.',
    'internal/shared/sheet': 'قارئ ملفات Excel وCSV الحقيقية بكل صيغها القديمة والجديدة.',
    'internal/shared/i18n': 'كل نصوص الواجهة بالعربي والإنجليزي، مقسمة على ملفات حسب الشاشات.',
    'internal/shared/matchflow': 'العقد المشترك لمرحلة المطابقة بالذكاء الاصطناعي بعد المحرك الحتمي.',
    'internal/modules': 'موديولات الأعمال. كل موديول سياق مستقل يقابل مخطط في قاعدة البيانات، وله غالبًا: domain (النماذج)، service (المنطق)، repository (عقد التخزين)، postgres (الاستعلامات)، http (مسارات API).',
    'internal/modules/assistant': 'المساعد الذكي "كبسولة": المحادثات، الأدوات اللي بتقرا البيانات حسب صلاحية المستخدم، الاقتراحات اللي المستخدم بيأكدها، والتصدير.',
    'internal/modules/assistant/tools': 'أدوات المساعد. كل أداة مربوطة بصلاحية، والموديل ما يقدرش يقرا غير من خلالها.',
    'internal/modules/assistant/datasets': 'محرك استعلامات محكوم يسمح للمساعد يسأل أسئلة مرنة بأمان.',
    'internal/modules/assistant/actions': 'الإجراءات: الموديل يقترح، والمستخدم يأكد.',
    'internal/modules/catalog': 'الكتالوج الرئيسي ومتغيرات الموردين (سعر ومخزون كل مورد لكل منتج) واستيراد الكتالوج من الإدارة.',
    'internal/modules/commerce': 'السلة، الدفع، الطلبات، الشحنات، المندوبين، وحصص الفروع. فيه قاعدة الشراء الموحدة CheckAvailability.',
    'internal/modules/org': 'المنشآت (صيدلية أو مورد)، الفروع، الأعضاء، الأدوار، الاعتماد، التوصيل، والأعمال المؤسسية.',
    'internal/modules/identity': 'المستخدمين، الدخول، الجلسات، التحقق بخطوتين، والملف الشخصي.',
    'internal/modules/billing': 'المحافظ، الاشتراكات والخطط، الفواتير، الإيداع والسحب.',
    'internal/modules/promo': 'العروض الخاصة، باقات الرعاية ورصيدها، والإعلانات.',
    'internal/modules/ingest': 'استيراد المورد لكتالوجه ومخزونه من ملف، بمعالج متعدد الخطوات.',
    'internal/modules/compare': 'مقارنة أسعار الموردين وخصومات السوق والتقارير الاستراتيجية (المستودعات المؤقتة).',
    'internal/modules/smartorder': 'الطلب الذكي: الصيدلية ترفع قائمة نواقصها والنظام يطابقها ويختار أفضل مورد لكل بند.',
    'internal/modules/inventory': 'المستودعات والمخزون والتحويلات.',
    'internal/modules/workflow': 'التغطية الأسبوعية للموردين ومحرك أولويات الشراء.',
    'internal/modules/notifications': 'الإشعارات داخل التطبيق وسجل أحداثها.',
    'internal/modules/platform_admin': 'إعدادات المنصة، الترجمات، SEO، سجل التدقيق، الأخطاء، كونسول SQL، وسلة المحذوفات.',
    'internal/modules/attachments': 'المستندات والملفات ومراجعتها.',
    'internal/modules/hr': 'الموظفين والوظائف.',
    'internal/modules/chat': 'الرسائل بين المشتري والمورد.',
    'internal/modules/aicapabilities': 'قدرات الذكاء الاصطناعي المساعدة للمطابقة.',
    'internal/modules/chatbridge': 'المسار المشترك لقنوات الدردشة. كل قرار أمني يخص تيليجرام وواتساب موجود هنا مرة واحدة.',
    'internal/modules/telegram': 'جسر تيليجرام: الربط، البوت، الإشعارات. n8n بس بينقل الرسائل.',
    'internal/modules/whatsapp': 'جسر واتساب بنفس وظائف تيليجرام، مع قاعدة نافذة الـ 24 ساعة والقوالب.',
    'internal/modules/marketing': 'ملخص العروض الجديدة اللي ورك فلو السوشيال ميديا بيقراه.',
    'internal/modules/etl': 'منطق نقل البيانات من النظام القديم.',
    'internal/ui': 'الواجهة كلها بتترسم على السيرفر. ملفات *_handlers.go بتجهز البيانات وتختار القالب، والقوالب في pages وcomponents وlayouts.',
    'internal/ui/pages': 'قوالب الصفحات بصيغة templ. كل ملف .templ له ملف _templ.go مولد تلقائيًا. التعديل دايمًا في .templ وبعدين templ generate.',
    'internal/ui/components': 'المكونات المشتركة: الأيقونات، النوافذ، الترقيم، الشارات، ودرج المساعد كبسولة.',
    'internal/ui/layouts': 'هياكل الصفحات: الأساسي، ولوحة الإدارة، والمورد، والصيدلية، والقائمة الجانبية.',
    'internal/ui/static': 'الملفات الثابتة المضمنة في البرنامج: CSS وJavaScript والصور.',
    'internal/ui/static/css': 'الأنماط. tokens.css فيه الألوان والمسافات، وcomponents.css فيه المكونات المعيارية.',
    'internal/ui/static/js': 'سكربتات صغيرة فوق HTMX وAlpine.',
    'test': 'اختبارات خاصة: دقة المطابقة على ملفات حقيقية، واختبارات الحمل.'
  };

  var KIND = { src: 'مصدر', test: 'اختبار', generated: 'مولَّد', migration: 'migration', doc: 'وثيقة', group: 'مجموعة' };

  // ---------- theme ----------
  var root = document.documentElement;
  function storedTheme() {
    try { return localStorage.getItem('dawa-docs-theme'); } catch (e) { return null; }
  }
  function applyTheme(t) {
    if (t === 'light' || t === 'dark') root.setAttribute('data-theme', t);
    else root.removeAttribute('data-theme');
    var label = document.getElementById('theme-label');
    if (label) label.textContent = t === 'dark' ? 'الوضع الداكن' : t === 'light' ? 'الوضع الفاتح' : 'حسب النظام';
  }
  applyTheme(storedTheme());
  var themeBtn = document.getElementById('theme-btn');
  if (themeBtn) {
    themeBtn.addEventListener('click', function () {
      var cur = storedTheme();
      var next = cur === null ? 'dark' : cur === 'dark' ? 'light' : null;
      try { if (next) localStorage.setItem('dawa-docs-theme', next); else localStorage.removeItem('dawa-docs-theme'); } catch (e) {}
      applyTheme(next);
    });
  }

  // ---------- mobile nav ----------
  var menuBtn = document.getElementById('menu-btn');
  if (menuBtn) menuBtn.addEventListener('click', function () { document.body.classList.toggle('nav-open'); });
  document.querySelectorAll('.sidebar a').forEach(function (a) {
    a.addEventListener('click', function () { document.body.classList.remove('nav-open'); });
  });

  // ---------- active section ----------
  var links = Array.prototype.slice.call(document.querySelectorAll('.nav-link'));
  var byId = {};
  links.forEach(function (l) { byId[l.getAttribute('href').slice(1)] = l; });
  if ('IntersectionObserver' in window) {
    var obs = new IntersectionObserver(function (entries) {
      entries.forEach(function (en) {
        if (en.isIntersecting && byId[en.target.id]) {
          links.forEach(function (l) { l.classList.remove('active'); });
          byId[en.target.id].classList.add('active');
        }
      });
    }, { rootMargin: '-70px 0px -70% 0px' });
    Object.keys(byId).forEach(function (id) { var el = document.getElementById(id); if (el) obs.observe(el); });
  }

  // ---------- explorer ----------
  var FILES = window.DAWA_FILES || [];
  var tree = document.getElementById('tree');
  var search = document.getElementById('file-search');
  var count = document.getElementById('explorer-count');
  if (!tree) return;

  function dirOf(p) {
    if (p.slice(-2) === '/*') p = p.slice(0, -2) + '/x';
    var i = p.lastIndexOf('/');
    return i < 0 ? '.' : p.slice(0, i);
  }
  function baseOf(p) {
    var i = p.lastIndexOf('/');
    return i < 0 ? p : p.slice(i + 1);
  }
  function describeDir(d) {
    if (DIRS[d]) return DIRS[d];
    var parent = d;
    while (parent.indexOf('/') > 0) {
      parent = parent.slice(0, parent.lastIndexOf('/'));
      if (DIRS[parent]) {
        var leaf = d.slice(d.lastIndexOf('/') + 1);
        var hint = leaf === 'postgres' ? 'استعلامات PostgreSQL وتنفيذ التخزين' :
          leaf === 'http' ? 'مسارات الـ API الخاصة بالموديل' :
          leaf === 'jobs' ? 'المهام الخلفية' : leaf === 'pipeline' ? 'مراحل المعالجة' :
          leaf === 'testdata' ? 'بيانات الاختبارات' : 'مجلد فرعي';
        return hint + ' ضمن: ' + parent + '.';
      }
    }
    return '';
  }

  var filter = 'all';
  var groups = {};
  FILES.forEach(function (f) {
    var d = dirOf(f.p);
    (groups[d] = groups[d] || []).push(f);
  });
  var dirNames = Object.keys(groups).sort(function (a, b) {
    if (a === '.') return -1;
    if (b === '.') return 1;
    return a < b ? -1 : 1;
  });

  function matches(f) {
    if (filter === 'all') return true;
    if (filter === 'src') return f.k === 'src' || f.k === 'group';
    return f.k === filter;
  }

  function fileRow(f, showPath) {
    var row = document.createElement('div');
    row.className = 'file';
    var name = document.createElement('div');
    name.className = 'file-name';
    name.textContent = f.p.slice(-2) === '/*' ? baseOf(f.p.slice(0, -2)) + '/*' : baseOf(f.p);
    var desc = document.createElement('div');
    desc.className = 'file-desc';
    if (showPath) {
      var path = document.createElement('span');
      path.className = 'result-path';
      path.textContent = f.p;
      desc.appendChild(path);
    }
    desc.appendChild(document.createTextNode(f.d));
    if (f.k !== 'src') {
      var tag = document.createElement('span');
      tag.className = 'tag';
      tag.textContent = KIND[f.k] || f.k;
      desc.appendChild(tag);
    }
    var lines = document.createElement('div');
    lines.className = 'file-lines';
    lines.textContent = f.n ? f.n.toLocaleString('ar-EG') + ' سطر' : '';
    row.appendChild(name); row.appendChild(desc); row.appendChild(lines);
    return row;
  }

  function renderTree() {
    tree.innerHTML = '';
    var total = 0;
    dirNames.forEach(function (d) {
      var items = groups[d].filter(matches);
      if (!items.length) return;
      total += items.length;
      var det = document.createElement('details');
      det.className = 'dir';
      var sum = document.createElement('summary');
      var path = document.createElement('span');
      path.className = 'dir-path';
      path.textContent = d === '.' ? '/ (الجذر)' : d + '/';
      var c = document.createElement('span');
      c.className = 'dir-count';
      c.textContent = items.length.toLocaleString('ar-EG') + ' ملف';
      sum.appendChild(path); sum.appendChild(c);
      var dd = describeDir(d);
      if (dd) {
        var p = document.createElement('p');
        p.className = 'dir-desc';
        p.textContent = dd;
        sum.appendChild(p);
      }
      det.appendChild(sum);
      det.addEventListener('toggle', function () {
        if (det.open && det.children.length === 1) {
          var frag = document.createDocumentFragment();
          items.forEach(function (f) { frag.appendChild(fileRow(f, false)); });
          det.appendChild(frag);
        }
      });
      tree.appendChild(det);
    });
    count.textContent = total.toLocaleString('ar-EG') + ' ملف في ' + tree.children.length.toLocaleString('ar-EG') + ' مجلد';
  }

  function renderSearch(q) {
    var terms = q.toLowerCase().split(/\s+/).filter(Boolean);
    var hits = FILES.filter(function (f) {
      if (!matches(f)) return false;
      var hay = (f.p + ' ' + f.d).toLowerCase();
      return terms.every(function (t) { return hay.indexOf(t) >= 0; });
    });
    tree.innerHTML = '';
    var box = document.createElement('div');
    box.className = 'dir';
    var limit = Math.min(hits.length, 400);
    for (var i = 0; i < limit; i++) box.appendChild(fileRow(hits[i], true));
    if (!hits.length) {
      var e = document.createElement('div');
      e.className = 'empty';
      e.textContent = 'مفيش ملفات مطابقة. جرّب كلمة تانية بالعربي أو جزء من اسم الملف.';
      box.appendChild(e);
    }
    tree.appendChild(box);
    count.textContent = hits.length.toLocaleString('ar-EG') + ' نتيجة' + (hits.length > limit ? ' (معروض أول ' + limit + ')' : '');
  }

  function refresh() {
    var q = search.value.trim();
    if (q.length >= 2) renderSearch(q); else renderTree();
  }

  var timer;
  search.addEventListener('input', function () { clearTimeout(timer); timer = setTimeout(refresh, 140); });
  document.querySelectorAll('.chip').forEach(function (chip) {
    chip.addEventListener('click', function () {
      filter = chip.getAttribute('data-filter');
      document.querySelectorAll('.chip').forEach(function (c) { c.setAttribute('aria-pressed', c === chip ? 'true' : 'false'); });
      refresh();
    });
  });
  refresh();

  var toTop = document.getElementById('to-top');
  if (toTop) toTop.addEventListener('click', function () { window.scrollTo(0, 0); });
})();
