-- Categories (#35). kind is expense, income, or any (only the built-in
-- Uncategorized). Categories are archived, never deleted.
CREATE TABLE categories (
    id       INTEGER PRIMARY KEY,
    name     TEXT    NOT NULL,
    hint     TEXT    NOT NULL,
    kind     TEXT    NOT NULL,
    archived INTEGER NOT NULL DEFAULT 0,
    builtin  INTEGER NOT NULL DEFAULT 0
);

INSERT INTO categories (name, hint, kind, builtin) VALUES
    ('Uncategorized', 'نامشخص', 'any', 1),
    ('Food', 'غذا، ناهار، شام، رستوران', 'expense', 0),
    ('Snacks', 'تنقلات، خوراکی، شیرینی، کافه', 'expense', 0),
    ('Groceries', 'نان، سوپرمارکت، میوه، خرید خانه', 'expense', 0),
    ('Transport', 'تاکسی، اسنپ، مترو، اتوبوس', 'expense', 0),
    ('Fuel', 'بنزین، سوخت', 'expense', 0),
    ('Education', 'کلاس، دوره، کتاب، دانشگاه', 'expense', 0),
    ('Entertainment', 'سینما، نتفلیکس، اشتراک، تفریح', 'expense', 0),
    ('Health', 'داروخانه، دکتر، دارو، درمان', 'expense', 0),
    ('Bills', 'قبض برق، آب، گاز، موبایل، اینترنت', 'expense', 0),
    ('Loan installment', 'قسط، وام', 'expense', 0),
    ('Shopping', 'لباس، دیجی‌کالا، خرید', 'expense', 0),
    ('Cash withdrawal', 'برداشت نقدی', 'expense', 0),
    ('Rent', 'اجاره', 'expense', 0),
    ('Gifts', 'هدیه، کادو', 'expense', 0),
    ('Other', 'سایر', 'expense', 0),
    ('Salary', 'حقوق', 'income', 0),
    ('Other income', 'سایر درآمد، بازگشت وجه', 'income', 0);
