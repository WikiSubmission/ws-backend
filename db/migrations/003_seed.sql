INSERT INTO languages (code, name, direction) VALUES 
('ar', 'Arabic', 'rtl'),
('ac', 'Arabic Clean', 'rtl'),
('en', 'English', 'ltr'),
('fr', 'French', 'ltr'),
('tr', 'Turkish', 'ltr'),
('de', 'German', 'ltr'),
('id', 'Bahasa', 'ltr'),
('fa', 'Persian', 'rtl'),
('ta', 'Tamil', 'ltr'),
('sv', 'Swedish', 'ltr'),
('ru', 'Russian', 'ltr'),
('bn', 'Bengali', 'ltr'),
('ur', 'Urdu', 'rtl'),
('es', 'Spanish', 'ltr'),
('tl', 'Transliterated', 'ltr')
ON CONFLICT (code) DO NOTHING;

-----------------------------------------
-- Cluster
-----------------------------------------
CLUSTER verses USING idx_verses_nav;