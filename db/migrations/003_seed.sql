INSERT INTO languages (code, name, direction) VALUES 
('ar', 'Arabic', 'rtl'),
('en', 'English', 'ltr'),
('fr', 'French', 'ltr');


-----------------------------------------
-- Cluster
-----------------------------------------
CLUSTER verses USING idx_verses_nav;