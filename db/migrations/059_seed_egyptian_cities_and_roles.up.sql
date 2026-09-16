-- Migration 059: Seed Egyptian Cities & Organization Role Parity
-- Seeds all 125 Egyptian cities & districts with geographical coordinates and adds org_pharmacist role.

BEGIN;

-- 1. Ensure Egypt Country and Egyptian Cities Seed Data Exist
DO $$
DECLARE
    v_country_id BIGINT;
BEGIN
    INSERT INTO platform_admin.countries (code, name, phone_code, currency, is_active)
    VALUES ('EG', '{"ar":"مصر","en":"Egypt"}'::jsonb, '+20', 'EGP', true)
    ON CONFLICT (code) DO UPDATE SET is_active = true
    RETURNING id INTO v_country_id;

    IF v_country_id IS NULL THEN
        SELECT id INTO v_country_id FROM platform_admin.countries WHERE code = 'EG' LIMIT 1;
    END IF;

    -- Seed all 125 Egyptian Cities / Districts (100% Laravel Parity)
    INSERT INTO platform_admin.cities (country_id, latitude, longitude, population, area_km2, time_zone, is_capital, is_active, name, region) VALUES
        (v_country_id, 30.0444, 31.2357, 9801536, 60611, 'Africa/Cairo', true, true, '{"ar":"القاهرة", "en":"Cairo", "ru":"Каир"}'::jsonb, '{"ar":"القاهرة", "en":"Cairo", "ru":"Каир"}'::jsonb),
        (v_country_id, 30.0131, 31.2089, 319488, 34251, 'Africa/Cairo', false, true, '{"ar":"القاهرة الجديدة", "en":"New Cairo", "ru":"Новый Каир"}'::jsonb, '{"ar":"القاهرة", "en":"Cairo", "ru":"Каир"}'::jsonb),
        (v_country_id, 30.1219, 31.3665, 93759, 17490, 'Africa/Cairo', false, true, '{"ar":"الشروق", "en":"Al-Shorouk", "ru":"Аш-Шурук"}'::jsonb, '{"ar":"القاهرة", "en":"Cairo", "ru":"Каир"}'::jsonb),
        (v_country_id, 30.1842, 31.2482, 33602, 10346, 'Africa/Cairo', false, true, '{"ar":"مدينة بدر", "en":"Badr City", "ru":"Город Бадр"}'::jsonb, '{"ar":"القاهرة", "en":"Cairo", "ru":"Каир"}'::jsonb),
        (v_country_id, 31.2001, 29.9187, 5362517, 110755, 'Africa/Cairo', false, true, '{"ar":"الإسكندرية", "en":"Alexandria", "ru":"Александрия"}'::jsonb, '{"ar":"الإسكندرية", "en":"Alexandria", "ru":"Александрия"}'::jsonb),
        (v_country_id, 30.9164, 29.5553, 46727, 16621, 'Africa/Cairo', false, true, '{"ar":"مدينة برج العرب الجديدة", "en":"New Borg El Arab", "ru":"Новый Борг-эль-Араб"}'::jsonb, '{"ar":"الإسكندرية", "en":"Alexandria", "ru":"Александрия"}'::jsonb),
        (v_country_id, 31.0333, 29.7667, 22866, 2842, 'Africa/Cairo', false, true, '{"ar":"برج العرب", "en":"Borg El Arab", "ru":"Борг-эль-Араб"}'::jsonb, '{"ar":"الإسكندرية", "en":"Alexandria", "ru":"Александрия"}'::jsonb),
        (v_country_id, 30.0131, 31.2089, 4458135, 9840, 'Africa/Cairo', false, true, '{"ar":"الجيزة", "en":"Giza", "ru":"Гиза"}'::jsonb, '{"ar":"الجيزة", "en":"Giza", "ru":"Гиза"}'::jsonb),
        (v_country_id, 30.0648, 30.9706, 376302, 43753, 'Africa/Cairo', false, true, '{"ar":"مدينة ستة أكتوبر", "en":"6th October City", "ru":"Город 6 октября"}'::jsonb, '{"ar":"الجيزة", "en":"Giza", "ru":"Гиза"}'::jsonb),
        (v_country_id, 30.1111, 30.8544, 97848, 4313, 'Africa/Cairo', false, true, '{"ar":"الشيخ زايد", "en":"Sheikh Zayed City", "ru":"Город Шейх Заид"}'::jsonb, '{"ar":"الجيزة", "en":"Giza", "ru":"Гиза"}'::jsonb),
        (v_country_id, 29.9667, 31.3, 158278, 1039, 'Africa/Cairo', false, true, '{"ar":"الحوامدية", "en":"Al-Hawamidiya", "ru":"Аль-Хавамидия"}'::jsonb, '{"ar":"الجيزة", "en":"Giza", "ru":"Гиза"}'::jsonb),
        (v_country_id, 29.8833, 31.2333, 96132, 1246, 'Africa/Cairo', false, true, '{"ar":"أوسيم", "en":"Ausim", "ru":"Аусим"}'::jsonb, '{"ar":"الجيزة", "en":"Giza", "ru":"Гиза"}'::jsonb),
        (v_country_id, 29.8167, 31.2833, 92724, 1332, 'Africa/Cairo', false, true, '{"ar":"البدرشين", "en":"Al-Badrashein", "ru":"Аль-Бадрашейн"}'::jsonb, '{"ar":"الجيزة", "en":"Giza", "ru":"Гиза"}'::jsonb),
        (v_country_id, 30.45, 31.1833, 1275700, 2999, 'Africa/Cairo', false, true, '{"ar":"شبرا الخيمة", "en":"Shubra El-Kheima", "ru":"Шубра-эль-Хейма"}'::jsonb, '{"ar":"القليوبية", "en":"Qalyubia", "ru":"Калюбия"}'::jsonb),
        (v_country_id, 30.4667, 31.1833, 502864, 773, 'Africa/Cairo', false, true, '{"ar":"الخصوص", "en":"Al-Khosous", "ru":"Аль-Хосус"}'::jsonb, '{"ar":"القليوبية", "en":"Qalyubia", "ru":"Калюбия"}'::jsonb),
        (v_country_id, 30.4667, 31.1833, 187469, 1095, 'Africa/Cairo', false, true, '{"ar":"بنها", "en":"Benha", "ru":"Бенха"}'::jsonb, '{"ar":"القليوبية", "en":"Qalyubia", "ru":"Калюбия"}'::jsonb),
        (v_country_id, 30.1833, 31.2167, 160831, 2143, 'Africa/Cairo', false, true, '{"ar":"قليوب", "en":"Qalyub", "ru":"Калюб"}'::jsonb, '{"ar":"القليوبية", "en":"Qalyubia", "ru":"Калюбия"}'::jsonb),
        (v_country_id, 30.2, 31.3167, 142955, 20923, 'Africa/Cairo', false, true, '{"ar":"العبور", "en":"El Obour", "ru":"Эль-Убур"}'::jsonb, '{"ar":"القليوبية", "en":"Qalyubia", "ru":"Калюбия"}'::jsonb),
        (v_country_id, 31.2654, 32.302, 791749, 129425, 'Africa/Cairo', false, true, '{"ar":"بور سعيد", "en":"Port Said", "ru":"Порт-Саид"}'::jsonb, '{"ar":"بور سعيد", "en":"Port Said", "ru":"Порт-Саид"}'::jsonb),
        (v_country_id, 29.9737, 32.5263, 716458, 312788, 'Africa/Cairo', false, true, '{"ar":"السويس", "en":"Suez", "ru":"Суэц"}'::jsonb, '{"ar":"السويس", "en":"Suez", "ru":"Суэц"}'::jsonb),
        (v_country_id, 31.0364, 31.3803, 632330, 2822, 'Africa/Cairo', false, true, '{"ar":"المنصورة", "en":"Al Mansoura", "ru":"Мансура"}'::jsonb, '{"ar":"الدقهلية", "en":"Dakahlia", "ru":"Дакахлия"}'::jsonb),
        (v_country_id, 31.1667, 31.4167, 164743, 233, 'Africa/Cairo', false, true, '{"ar":"المطرية", "en":"Al Matariya", "ru":"Матария"}'::jsonb, '{"ar":"الدقهلية", "en":"Dakahlia", "ru":"Дакахлия"}'::jsonb),
        (v_country_id, 31.25, 31.4167, 156319, 1330, 'Africa/Cairo', false, true, '{"ar":"ميت غمر", "en":"Mit Ghamr", "ru":"Мит-Гамр"}'::jsonb, '{"ar":"الدقهلية", "en":"Dakahlia", "ru":"Дакахлия"}'::jsonb),
        (v_country_id, 31.1944, 31.3111, 139369, 3251, 'Africa/Cairo', false, true, '{"ar":"بلقاس", "en":"Belqas", "ru":"Белкас"}'::jsonb, '{"ar":"الدقهلية", "en":"Dakahlia", "ru":"Дакахлия"}'::jsonb),
        (v_country_id, 31.1167, 31.4, 129519, 5016, 'Africa/Cairo', false, true, '{"ar":"المنزلة", "en":"Al Manzala", "ru":"Манзала"}'::jsonb, '{"ar":"الدقهلية", "en":"Dakahlia", "ru":"Дакахлия"}'::jsonb),
        (v_country_id, 30.5883, 31.5017, 460501, 1403, 'Africa/Cairo', false, true, '{"ar":"الزقازيق", "en":"Zagazig", "ru":"Загазиг"}'::jsonb, '{"ar":"الشرقية", "en":"Sharqia", "ru":"Шаркия"}'::jsonb),
        (v_country_id, 30.4167, 31.6, 263321, 6605, 'Africa/Cairo', false, true, '{"ar":"مدينة عشرة رمضان", "en":"10th of Ramadan City", "ru":"Город 10 рамадана"}'::jsonb, '{"ar":"الشرقية", "en":"Sharqia", "ru":"Шаркия"}'::jsonb),
        (v_country_id, 30.4167, 31.5667, 198167, 3486, 'Africa/Cairo', false, true, '{"ar":"بلبيس", "en":"Bilbays", "ru":"Билбайс"}'::jsonb, '{"ar":"الشرقية", "en":"Sharqia", "ru":"Шаркия"}'::jsonb),
        (v_country_id, 30.6833, 31.4333, 165246, 2938, 'Africa/Cairo', false, true, '{"ar":"أبو كبير", "en":"Abu Kabir", "ru":"Абу-Кабир"}'::jsonb, '{"ar":"الشرقية", "en":"Sharqia", "ru":"Шаркия"}'::jsonb),
        (v_country_id, 30.5852, 32.2654, 450388, 6885, 'Africa/Cairo', false, true, '{"ar":"الإسماعيلية", "en":"Ismailia", "ru":"Исмаилия"}'::jsonb, '{"ar":"الإسماعيلية", "en":"Ismailia", "ru":"Исмаилия"}'::jsonb),
        (v_country_id, 30.6167, 32.2833, 49251, 1856, 'Africa/Cairo', false, true, '{"ar":"التل الكبير", "en":"Al-Tall Al-Kabir", "ru":"Тель-эль-Кебир"}'::jsonb, '{"ar":"الإسماعيلية", "en":"Ismailia", "ru":"Исмаилия"}'::jsonb),
        (v_country_id, 30.3167, 32.3, 49906, 634, 'Africa/Cairo', false, true, '{"ar":"القنطرة", "en":"Al-Qantara", "ru":"Кантара"}'::jsonb, '{"ar":"الإسماعيلية", "en":"Ismailia", "ru":"Исмаилия"}'::jsonb),
        (v_country_id, 31.4165, 31.8133, 312863, 353, 'Africa/Cairo', false, true, '{"ar":"دمياط", "en":"Damietta", "ru":"Дамьетта"}'::jsonb, '{"ar":"دمياط", "en":"Damietta", "ru":"Дамьетта"}'::jsonb),
        (v_country_id, 31.4167, 31.7333, 55749, 4394, 'Africa/Cairo', false, true, '{"ar":"دمياط الجديدة", "en":"New Damietta", "ru":"Новая Дамьетта"}'::jsonb, '{"ar":"دمياط", "en":"Damietta", "ru":"Дамьетта"}'::jsonb),
        (v_country_id, 31.3333, 31.75, 44390, 1413, 'Africa/Cairo', false, true, '{"ar":"فارسكور", "en":"Faraskur", "ru":"Фараскур"}'::jsonb, '{"ar":"دمياط", "en":"Damietta", "ru":"Дамьетта"}'::jsonb),
        (v_country_id, 31.1, 30.9167, 198197, 1934, 'Africa/Cairo', false, true, '{"ar":"كفر الشيخ", "en":"Kafr El Sheikh", "ru":"Кафр-эш-Шейх"}'::jsonb, '{"ar":"كفر الشيخ", "en":"Kafr El Sheikh", "ru":"Кафр-эш-Шейх"}'::jsonb),
        (v_country_id, 31.1667, 30.9333, 152077, 864, 'Africa/Cairo', false, true, '{"ar":"دسوق", "en":"Desouk", "ru":"Десук"}'::jsonb, '{"ar":"كفر الشيخ", "en":"Kafr El Sheikh", "ru":"Кафр-эш-Шейх"}'::jsonb),
        (v_country_id, 31.35, 30.7667, 95134, 2395, 'Africa/Cairo', false, true, '{"ar":"فوه", "en":"Fuwah", "ru":"Фува"}'::jsonb, '{"ar":"كفر الشيخ", "en":"Kafr El Sheikh", "ru":"Кафр-эш-Шейх"}'::jsonb),
        (v_country_id, 30.7833, 31, 597694, 1953, 'Africa/Cairo', false, true, '{"ar":"طنطا", "en":"Tanta", "ru":"Танта"}'::jsonb, '{"ar":"الغربية", "en":"Gharbia", "ru":"Гарбия"}'::jsonb),
        (v_country_id, 30.9764, 31.1669, 614202, 1891, 'Africa/Cairo', false, true, '{"ar":"المحلة الكبرى", "en":"Al Mahalla Al Kubra", "ru":"Махалла-эль-Кубра"}'::jsonb, '{"ar":"الغربية", "en":"Gharbia", "ru":"Гарбия"}'::jsonb),
        (v_country_id, 30.8167, 30.9333, 115774, 1624, 'Africa/Cairo', false, true, '{"ar":"زفتى", "en":"Zifta", "ru":"Зифта"}'::jsonb, '{"ar":"الغربية", "en":"Gharbia", "ru":"Гарбия"}'::jsonb),
        (v_country_id, 31.0333, 30.4667, 329572, 684, 'Africa/Cairo', false, true, '{"ar":"دمنهور", "en":"Damanhour", "ru":"Даманхур"}'::jsonb, '{"ar":"البحيرة", "en":"Beheira", "ru":"Бехейра"}'::jsonb),
        (v_country_id, 31.0333, 30.3667, 183479, 7244, 'Africa/Cairo', false, true, '{"ar":"إدكو", "en":"Edko", "ru":"Эдко"}'::jsonb, '{"ar":"البحيرة", "en":"Beheira", "ru":"Бехейра"}'::jsonb),
        (v_country_id, 31.1333, 30.1333, 133130, 546, 'Africa/Cairo', false, true, '{"ar":"كفر الدوار", "en":"Kafr El Dawwar", "ru":"Кафр-эд-Даввар"}'::jsonb, '{"ar":"البحيرة", "en":"Beheira", "ru":"Бехейра"}'::jsonb),
        (v_country_id, 31.3167, 29.8167, 130270, 9228, 'Africa/Cairo', false, true, '{"ar":"رشيد", "en":"Rosetta", "ru":"Розетта"}'::jsonb, '{"ar":"البحيرة", "en":"Beheira", "ru":"Бехейра"}'::jsonb),
        (v_country_id, 30.5667, 31, 277991, 2852, 'Africa/Cairo', false, true, '{"ar":"شبين الكوم", "en":"Shibin El Kom", "ru":"Шибин-эль-Ком"}'::jsonb, '{"ar":"المنوفية", "en":"Monufia", "ru":"Минуфия"}'::jsonb),
        (v_country_id, 30.6167, 30.95, 130417, 1741, 'Africa/Cairo', false, true, '{"ar":"منوف", "en":"Menuf", "ru":"Менуф"}'::jsonb, '{"ar":"المنوفية", "en":"Monufia", "ru":"Минуфия"}'::jsonb),
        (v_country_id, 30.6333, 30.9833, 129144, 2335, 'Africa/Cairo', false, true, '{"ar":"أشمون", "en":"Ashmoun", "ru":"Ашмун"}'::jsonb, '{"ar":"المنوفية", "en":"Monufia", "ru":"Минуфия"}'::jsonb),
        (v_country_id, 30.4667, 31.1333, 74177, 1456, 'Africa/Cairo', false, true, '{"ar":"مدينة السادات", "en":"Sadat City", "ru":"Город Садат"}'::jsonb, '{"ar":"المنوفية", "en":"Monufia", "ru":"Минуфия"}'::jsonb),
        (v_country_id, 29.3083, 30.8428, 531861, 1847, 'Africa/Cairo', false, true, '{"ar":"الفيوم", "en":"Fayoum", "ru":"Файюм"}'::jsonb, '{"ar":"الفيوم", "en":"Fayoum", "ru":"Файюм"}'::jsonb),
        (v_country_id, 29.1167, 30.8333, 136825, 1309, 'Africa/Cairo', false, true, '{"ar":"سنورس", "en":"Sinnuris", "ru":"Синнурис"}'::jsonb, '{"ar":"الفيوم", "en":"Fayoum", "ru":"Файюм"}'::jsonb),
        (v_country_id, 29.1, 30.9167, 88309, 2038, 'Africa/Cairo', false, true, '{"ar":"إبشواي", "en":"Ibshaway", "ru":"Ибшавай"}'::jsonb, '{"ar":"الفيوم", "en":"Fayoum", "ru":"Файюм"}'::jsonb),
        (v_country_id, 29.0661, 31.0994, 294125, 1400, 'Africa/Cairo', false, true, '{"ar":"بني سويف", "en":"Beni Suef", "ru":"Бени-Суэф"}'::jsonb, '{"ar":"بني سويف", "en":"Beni Suef", "ru":"Бени-Суэф"}'::jsonb),
        (v_country_id, 29.3333, 31.05, 146917, 3588, 'Africa/Cairo', false, true, '{"ar":"ناصر", "en":"Naser", "ru":"Насер"}'::jsonb, '{"ar":"بني سويف", "en":"Beni Suef", "ru":"Бени-Суэф"}'::jsonb),
        (v_country_id, 29.0833, 31.0167, 121675, 1610, 'Africa/Cairo', false, true, '{"ar":"الفشن", "en":"Al Fashn", "ru":"Аль-Фашн"}'::jsonb, '{"ar":"بني سويف", "en":"Beni Suef", "ru":"Бени-Суэф"}'::jsonb),
        (v_country_id, 28.1167, 30.75, 298021, 1498, 'Africa/Cairo', false, true, '{"ar":"المنيا", "en":"Al Minya", "ru":"Минья"}'::jsonb, '{"ar":"المنيا", "en":"Minia", "ru":"Минья"}'::jsonb),
        (v_country_id, 28.1167, 30.8333, 223435, 1929, 'Africa/Cairo', false, true, '{"ar":"ملوى", "en":"Mallawi", "ru":"Маллави"}'::jsonb, '{"ar":"المنيا", "en":"Minia", "ru":"Минья"}'::jsonb),
        (v_country_id, 27.9833, 30.8, 149227, 1393, 'Africa/Cairo', false, true, '{"ar":"سمالوط", "en":"Samalut", "ru":"Самалут"}'::jsonb, '{"ar":"المنيا", "en":"Minia", "ru":"Минья"}'::jsonb),
        (v_country_id, 27.1833, 31.1833, 562061, 2660, 'Africa/Cairo', false, true, '{"ar":"أسيوط", "en":"Asyut", "ru":"Асьют"}'::jsonb, '{"ar":"أسيوط", "en":"Asyut", "ru":"Асьют"}'::jsonb),
        (v_country_id, 27.3333, 31.1167, 125373, 1903, 'Africa/Cairo', false, true, '{"ar":"منفلوط", "en":"Manfalut", "ru":"Манфалут"}'::jsonb, '{"ar":"أسيوط", "en":"Asyut", "ru":"Асьют"}'::jsonb),
        (v_country_id, 27.05, 31.1667, 112077, 1386, 'Africa/Cairo', false, true, '{"ar":"أبو تيج", "en":"Abu Tig", "ru":"Абу-Тиг"}'::jsonb, '{"ar":"أسيوط", "en":"Asyut", "ru":"Асьют"}'::jsonb),
        (v_country_id, 26.55, 31.6944, 278425, 903, 'Africa/Cairo', false, true, '{"ar":"سوهاج", "en":"Sohag", "ru":"Сохаг"}'::jsonb, '{"ar":"سوهاج", "en":"Sohag", "ru":"Сохаг"}'::jsonb),
        (v_country_id, 26.7167, 31.65, 157938, 1473, 'Africa/Cairo', false, true, '{"ar":"أخميم", "en":"Akhmim", "ru":"Ахмим"}'::jsonb, '{"ar":"سوهاج", "en":"Sohag", "ru":"Сохаг"}'::jsonb),
        (v_country_id, 26.5833, 31.5, 157754, 1153, 'Africa/Cairo', false, true, '{"ar":"جرجا", "en":"Girga", "ru":"Гирга"}'::jsonb, '{"ar":"سوهاج", "en":"Sohag", "ru":"Сохаг"}'::jsonb),
        (v_country_id, 26.1551, 32.716, 264498, 3158, 'Africa/Cairo', false, true, '{"ar":"قنا", "en":"Qena", "ru":"Кена"}'::jsonb, '{"ar":"قنا", "en":"Qena", "ru":"Кена"}'::jsonb),
        (v_country_id, 25.9167, 32.7333, 88261, 1568, 'Africa/Cairo', false, true, '{"ar":"قوص", "en":"Qus", "ru":"Кус"}'::jsonb, '{"ar":"قنا", "en":"Qena", "ru":"Кена"}'::jsonb),
        (v_country_id, 26.0167, 32.8667, 74275, 613, 'Africa/Cairo', false, true, '{"ar":"فرشوط", "en":"Farshut", "ru":"Фаршут"}'::jsonb, '{"ar":"قنا", "en":"Qena", "ru":"Кена"}'::jsonb),
        (v_country_id, 25.6872, 32.6396, 284952, 4299, 'Africa/Cairo', false, true, '{"ar":"الأقصر", "en":"Luxor", "ru":"Луксор"}'::jsonb, '{"ar":"الأقصر", "en":"Luxor", "ru":"Луксор"}'::jsonb),
        (v_country_id, 25.5167, 32.5333, 102697, 1903, 'Africa/Cairo', false, true, '{"ar":"إسنا", "en":"Esna", "ru":"Эсна"}'::jsonb, '{"ar":"الأقصر", "en":"Luxor", "ru":"Луксор"}'::jsonb),
        (v_country_id, 25.6167, 32.55, 83475, 5508, 'Africa/Cairo', false, true, '{"ar":"أرمنت", "en":"Armant", "ru":"Армант"}'::jsonb, '{"ar":"الأقصر", "en":"Luxor", "ru":"Луксор"}'::jsonb),
        (v_country_id, 24.0889, 32.8998, 401890, 37512, 'Africa/Cairo', false, true, '{"ar":"أسوان", "en":"Aswan", "ru":"Асуан"}'::jsonb, '{"ar":"أسوان", "en":"Aswan", "ru":"Асуан"}'::jsonb),
        (v_country_id, 24.4833, 32.9333, 123066, 1520, 'Africa/Cairo', false, true, '{"ar":"كوم أمبو", "en":"Kom Ombo", "ru":"Ком-Омбо"}'::jsonb, '{"ar":"أسوان", "en":"Aswan", "ru":"Асуан"}'::jsonb),
        (v_country_id, 24.9833, 32.8833, 84140, 3609, 'Africa/Cairo', false, true, '{"ar":"إدفو", "en":"Edfu", "ru":"Эдфу"}'::jsonb, '{"ar":"أسوان", "en":"Aswan", "ru":"Асуан"}'::jsonb),
        (v_country_id, 27.2574, 33.8129, 214247, 25772, 'Africa/Cairo', false, true, '{"ar":"الغردقة", "en":"Hurghada", "ru":"Хургада"}'::jsonb, '{"ar":"البحر الأحمر", "en":"Red Sea", "ru":"Красное море"}'::jsonb),
        (v_country_id, 26.7333, 33.9167, 55487, 99464, 'Africa/Cairo', false, true, '{"ar":"سفاجا", "en":"Safaga", "ru":"Сафага"}'::jsonb, '{"ar":"البحر الأحمر", "en":"Red Sea", "ru":"Красное море"}'::jsonb),
        (v_country_id, 28.35, 33.0833, 52558, 6588, 'Africa/Cairo', false, true, '{"ar":"القصير", "en":"Al Qusayr", "ru":"Кусейр"}'::jsonb, '{"ar":"البحر الأحمر", "en":"Red Sea", "ru":"Красное море"}'::jsonb),
        (v_country_id, 25.4515, 30.5435, 87482, 25062, 'Africa/Cairo', false, true, '{"ar":"الخارجة", "en":"Kharga", "ru":"Харга"}'::jsonb, '{"ar":"الوادي الجديد", "en":"New Valley", "ru":"Новая долина"}'::jsonb),
        (v_country_id, 25.5, 29, 28229, 3534, 'Africa/Cairo', false, true, '{"ar":"الداخلة", "en":"Dakhla", "ru":"Дахла"}'::jsonb, '{"ar":"الوادي الجديد", "en":"New Valley", "ru":"Новая долина"}'::jsonb),
        (v_country_id, 31.3525, 27.2373, 193293, 65357, 'Africa/Cairo', false, true, '{"ar":"مرسى مطروح", "en":"Marsa Matruh", "ru":"Марса-Матрух"}'::jsonb, '{"ar":"مطروح", "en":"Matruh", "ru":"Матрух"}'::jsonb),
        (v_country_id, 31.6167, 25.1667, 41893, 59160, 'Africa/Cairo', false, true, '{"ar":"سيدي براني", "en":"Sidi Barrani", "ru":"Сиди-Баррани"}'::jsonb, '{"ar":"مطروح", "en":"Matruh", "ru":"Матрух"}'::jsonb),
        (v_country_id, 29.2, 25.5167, 27409, 20055, 'Africa/Cairo', false, true, '{"ar":"سيوة", "en":"Siwa", "ru":"Сива"}'::jsonb, '{"ar":"مطروح", "en":"Matruh", "ru":"Матрух"}'::jsonb),
        (v_country_id, 31.0333, 28.45, 30332, 7830, 'Africa/Cairo', false, true, '{"ar":"الضبعة", "en":"Al-Dabaa", "ru":"Дабаа"}'::jsonb, '{"ar":"مطروح", "en":"Matruh", "ru":"Матрух"}'::jsonb),
        (v_country_id, 31.1319, 33.7975, 204391, 30756, 'Africa/Cairo', false, true, '{"ar":"العريش", "en":"Al Arish", "ru":"Эль-Ариш"}'::jsonb, '{"ar":"شمال سيناء", "en":"North Sinai", "ru":"Северный Синай"}'::jsonb),
        (v_country_id, 31.05, 34.25, 45359, 5904, 'Africa/Cairo', false, true, '{"ar":"رفح", "en":"Rafah", "ru":"Рафах"}'::jsonb, '{"ar":"شمال سيناء", "en":"North Sinai", "ru":"Северный Синай"}'::jsonb),
        (v_country_id, 31.0167, 33.6167, 27406, 2845, 'Africa/Cairo', false, true, '{"ar":"الشيخ زويد", "en":"Sheikh Zuweid", "ru":"Шейх-Зувейд"}'::jsonb, '{"ar":"شمال سيناء", "en":"North Sinai", "ru":"Северный Синай"}'::jsonb),
        (v_country_id, 30.8333, 33.8333, 27012, 8020, 'Africa/Cairo', false, true, '{"ar":"بئر العبد", "en":"Bir Al-Abd", "ru":"Бир-аль-Абд"}'::jsonb, '{"ar":"شمال سيناء", "en":"North Sinai", "ru":"Северный Синай"}'::jsonb),
        (v_country_id, 28.2285, 33.8455, 40532, 6001, 'Africa/Cairo', false, true, '{"ar":"الطور", "en":"El Tor", "ru":"Эт-Тур"}'::jsonb, '{"ar":"جنوب سيناء", "en":"South Sinai", "ru":"Южный Синай"}'::jsonb),
        (v_country_id, 27.9158, 34.33, 25000, 4466, 'Africa/Cairo', false, true, '{"ar":"شرم الشيخ", "en":"Sharm El Sheikh", "ru":"Шарм-эш-Шейх"}'::jsonb, '{"ar":"جنوب سيناء", "en":"South Sinai", "ru":"Южный Синай"}'::jsonb),
        (v_country_id, 28.0333, 33.6167, 12849, 390976, 'Africa/Cairo', false, true, '{"ar":"الشلاتين", "en":"Shalatin", "ru":"Шалатин"}'::jsonb, '{"ar":"جنوب سيناء", "en":"South Sinai", "ru":"Южный Синай"}'::jsonb),
        (v_country_id, 29.85, 31.3, 950000, 2500, 'Africa/Cairo', false, true, '{"ar":"حلوان", "en":"Helwan", "ru":"Хелуан"}'::jsonb, '{"ar":"القاهرة", "en":"Cairo", "ru":"Каир"}'::jsonb),
        (v_country_id, 29.9333, 31.1333, 140448, 1953, 'Africa/Cairo', false, true, '{"ar":"كرداسة", "en":"Kerdasa", "ru":"Кердаса"}'::jsonb, '{"ar":"الجيزة", "en":"Giza", "ru":"Гиза"}'::jsonb),
        (v_country_id, 29.75, 31.2, 88401, 651, 'Africa/Cairo', false, true, '{"ar":"أبو النمرس", "en":"Abu Nomros", "ru":"Абу-Номрос"}'::jsonb, '{"ar":"الجيزة", "en":"Giza", "ru":"Гиза"}'::jsonb),
        (v_country_id, 29.7167, 31.2167, 62812, 2500, 'Africa/Cairo', false, true, '{"ar":"الصف", "en":"Al-Saff", "ru":"Ас-Сафф"}'::jsonb, '{"ar":"الجيزة", "en":"Giza", "ru":"Гиза"}'::jsonb),
        (v_country_id, 31.1167, 31.3833, 126088, 1883, 'Africa/Cairo', false, true, '{"ar":"السنبلاوين", "en":"Al-Senbellawein", "ru":"Сенбеллавейн"}'::jsonb, '{"ar":"الدقهلية", "en":"Dakahlia", "ru":"Дакахлия"}'::jsonb),
        (v_country_id, 31.1833, 31.3167, 114737, 1172, 'Africa/Cairo', false, true, '{"ar":"طلخا", "en":"Talkha", "ru":"Талха"}'::jsonb, '{"ar":"الدقهلية", "en":"Dakahlia", "ru":"Дакахлия"}'::jsonb),
        (v_country_id, 31.2, 31.2833, 102769, 2520, 'Africa/Cairo', false, true, '{"ar":"دكرنس", "en":"Dikirnis", "ru":"Дикирнис"}'::jsonb, '{"ar":"الدقهلية", "en":"Dakahlia", "ru":"Дакахлия"}'::jsonb),
        (v_country_id, 30.45, 31.7833, 125112, 1078, 'Africa/Cairo', false, true, '{"ar":"فاقوس", "en":"Faqous", "ru":"Факус"}'::jsonb, '{"ar":"الشرقية", "en":"Sharqia", "ru":"Шаркия"}'::jsonb),
        (v_country_id, 30.5833, 31.55, 106216, 628, 'Africa/Cairo', false, true, '{"ar":"منيا القمح", "en":"Menia Al-Qamh", "ru":"Менья-эль-Камх"}'::jsonb, '{"ar":"الشرقية", "en":"Sharqia", "ru":"Шаркия"}'::jsonb),
        (v_country_id, 30.6833, 31.6, 101238, 2358, 'Africa/Cairo', false, true, '{"ar":"القرين", "en":"Al-Qurein", "ru":"Аль-Курейн"}'::jsonb, '{"ar":"الشرقية", "en":"Sharqia", "ru":"Шаркия"}'::jsonb),
        (v_country_id, 30.7667, 30.9167, 91130, 1923, 'Africa/Cairo', false, true, '{"ar":"سمنود", "en":"Samanoud", "ru":"Саманунд"}'::jsonb, '{"ar":"الغربية", "en":"Gharbia", "ru":"Гарбия"}'::jsonb),
        (v_country_id, 30.8833, 30.9333, 86390, 919, 'Africa/Cairo', false, true, '{"ar":"كفر الزيات", "en":"Kafr El Zayat", "ru":"Кафр-эз-Зайят"}'::jsonb, '{"ar":"الغربية", "en":"Gharbia", "ru":"Гарбия"}'::jsonb),
        (v_country_id, 30.7333, 31, 77823, 1915, 'Africa/Cairo', false, true, '{"ar":"بسيون", "en":"Basyoun", "ru":"Басьюн"}'::jsonb, '{"ar":"الغربية", "en":"Gharbia", "ru":"Гарбия"}'::jsonb),
        (v_country_id, 30.9667, 30.4333, 85964, 2394, 'Africa/Cairo', false, true, '{"ar":"حوش عيسى", "en":"Hosh Issa", "ru":"Хош-Исса"}'::jsonb, '{"ar":"البحيرة", "en":"Beheira", "ru":"Бехейра"}'::jsonb),
        (v_country_id, 30.9833, 30.4167, 80377, 2564, 'Africa/Cairo', false, true, '{"ar":"إيتاي البارود", "en":"Itay El Baroud", "ru":"Итай-эль-Барунд"}'::jsonb, '{"ar":"البحيرة", "en":"Beheira", "ru":"Бехейра"}'::jsonb),
        (v_country_id, 31.1, 30.3833, 71361, 2204, 'Africa/Cairo', false, true, '{"ar":"الدلنجات", "en":"Al-Delengat", "ru":"Делинжат"}'::jsonb, '{"ar":"البحيرة", "en":"Beheira", "ru":"Бехейра"}'::jsonb),
        (v_country_id, 30.5167, 30.9667, 81097, 1615, 'Africa/Cairo', false, true, '{"ar":"سرس الليانة", "en":"Sers El-Lyan", "ru":"Серс-эль-Лиян"}'::jsonb, '{"ar":"المنوفية", "en":"Monufia", "ru":"Минуфия"}'::jsonb),
        (v_country_id, 30.55, 30.9833, 78919, 1892, 'Africa/Cairo', false, true, '{"ar":"الشهداء", "en":"Al-Shohada", "ru":"Шухада"}'::jsonb, '{"ar":"المنوفية", "en":"Monufia", "ru":"Минуфия"}'::jsonb),
        (v_country_id, 30.5833, 31.0333, 75255, 2299, 'Africa/Cairo', false, true, '{"ar":"تلا", "en":"Tala", "ru":"Тала"}'::jsonb, '{"ar":"المنوفية", "en":"Monufia", "ru":"Минуфия"}'::jsonb),
        (v_country_id, 29.1833, 30.9167, 76190, 2843, 'Africa/Cairo', false, true, '{"ar":"أطسا", "en":"Atsa", "ru":"Атса"}'::jsonb, '{"ar":"الفيوم", "en":"Fayoum", "ru":"Файюм"}'::jsonb),
        (v_country_id, 29.2167, 30.8833, 74870, 1484, 'Africa/Cairo', false, true, '{"ar":"طامية", "en":"Tamiya", "ru":"Тамия"}'::jsonb, '{"ar":"الفيوم", "en":"Fayoum", "ru":"Файюм"}'::jsonb),
        (v_country_id, 29.0833, 31.1333, 101946, 1713, 'Africa/Cairo', false, true, '{"ar":"ببا", "en":"Beba", "ru":"Беба"}'::jsonb, '{"ar":"بني سويف", "en":"Beni Suef", "ru":"Бени-Суэф"}'::jsonb),
        (v_country_id, 29.1, 31.0833, 69953, 1940, 'Africa/Cairo', false, true, '{"ar":"سمسطا", "en":"Somosta", "ru":"Сомоста"}'::jsonb, '{"ar":"بني سويف", "en":"Beni Suef", "ru":"Бени-Суэф"}'::jsonb),
        (v_country_id, 29.1167, 31.1167, 68976, 1111, 'Africa/Cairo', false, true, '{"ar":"إهناسيا", "en":"Ihnasya", "ru":"Ихнасия"}'::jsonb, '{"ar":"بني سويف", "en":"Beni Suef", "ru":"Бени-Суэф"}'::jsonb),
        (v_country_id, 28.0833, 30.7667, 124236, 1230, 'Africa/Cairo', false, true, '{"ar":"مغاغة", "en":"Maghagha", "ru":"Магага"}'::jsonb, '{"ar":"المنيا", "en":"Minia", "ru":"Минья"}'::jsonb),
        (v_country_id, 28.1833, 30.8167, 121643, 819, 'Africa/Cairo', false, true, '{"ar":"بني مزار", "en":"Bani Mazar", "ru":"Бени-Мазар"}'::jsonb, '{"ar":"المنيا", "en":"Minia", "ru":"Минья"}'::jsonb),
        (v_country_id, 28.05, 30.8333, 94855, 988, 'Africa/Cairo', false, true, '{"ar":"أبو قرقاص", "en":"Abu Qirqas", "ru":"Абу-Киркас"}'::jsonb, '{"ar":"المنيا", "en":"Minia", "ru":"Минья"}'::jsonb),
        (v_country_id, 27.1667, 31.1667, 118843, 1597, 'Africa/Cairo', false, true, '{"ar":"أبنوب", "en":"Abnub", "ru":"Абнуб"}'::jsonb, '{"ar":"أسيوط", "en":"Asyut", "ru":"Асьют"}'::jsonb),
        (v_country_id, 27.2833, 31.1833, 109051, 644, 'Africa/Cairo', false, true, '{"ar":"ديروط", "en":"Dairut", "ru":"Дайрут"}'::jsonb, '{"ar":"أسيوط", "en":"Asyut", "ru":"Асьют"}'::jsonb),
        (v_country_id, 27.0667, 31.2, 105891, 1258, 'Africa/Cairo', false, true, '{"ar":"القوصية", "en":"Al-Qusiya", "ru":"Кусия"}'::jsonb, '{"ar":"أسيوط", "en":"Asyut", "ru":"Асьют"}'::jsonb),
        (v_country_id, 26.7667, 31.5, 140087, 555, 'Africa/Cairo', false, true, '{"ar":"طهطا", "en":"Tahta", "ru":"Тахта"}'::jsonb, '{"ar":"سوهاج", "en":"Sohag", "ru":"Сохаг"}'::jsonb),
        (v_country_id, 26.4167, 31.4833, 105477, 1206, 'Africa/Cairo', false, true, '{"ar":"طما", "en":"Tima", "ru":"Тима"}'::jsonb, '{"ar":"سوهاج", "en":"Sohag", "ru":"Сохаг"}'::jsonb),
        (v_country_id, 26.3833, 31.5333, 95064, 1766, 'Africa/Cairo', false, true, '{"ar":"المنشأة", "en":"Al-Mansha", "ru":"Манша"}'::jsonb, '{"ar":"سوهاج", "en":"Sohag", "ru":"Сохаг"}'::jsonb),
        (v_country_id, 25.9833, 32.75, 67716, 1247, 'Africa/Cairo', false, true, '{"ar":"دشنا", "en":"Dishna", "ru":"Дишна"}'::jsonb, '{"ar":"قنا", "en":"Qena", "ru":"Кена"}'::jsonb),
        (v_country_id, 26.1167, 32.6833, 62339, 443, 'Africa/Cairo', false, true, '{"ar":"نجع حمادي", "en":"Nag Hammadi", "ru":"Наг-Хаммади"}'::jsonb, '{"ar":"قنا", "en":"Qena", "ru":"Кена"}'::jsonb),
        (v_country_id, 25.9333, 32.7167, 28006, 1038, 'Africa/Cairo', false, true, '{"ar":"نقادة", "en":"Naqada", "ru":"Накада"}'::jsonb, '{"ar":"قنا", "en":"Qena", "ru":"Кена"}'::jsonb)
    ON CONFLICT DO NOTHING;
END $$;

-- 2. Ensure Required Organization Role Keys Exist in identity.roles
INSERT INTO identity.roles (key, name, scope, is_system, description) VALUES
    ('org_pharmacist', '{"ar":"صيدلي مسؤول","en":"Responsible Pharmacist"}', 'organization', true, 'Responsible Pharmacist with branch & ordering capabilities')
ON CONFLICT (key) DO NOTHING;

COMMIT;
