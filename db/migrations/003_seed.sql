INSERT INTO languages (code, name, direction) VALUES 
('ar', 'Arabic', 'rtl'),
('en', 'English', 'ltr'),
('fr', 'French', 'ltr')
ON CONFLICT (code) DO NOTHING;;


-----------------------------------------
-- Cluster
-----------------------------------------
CLUSTER verses USING idx_verses_nav;