package sqltest

// gamesSchema mirrors the shipped games table definition verbatim.
const gamesSchema = "CREATE TABLE IF NOT EXISTS games (\n" +
	"  id INT NOT NULL default 0,\n" +
	"  idnr INT NOT NULL default 0,\n" +
	"  number1 INT NOT NULL default 0,\n" +
	"  number2 INT NOT NULL default 0,\n" +
	"  prize  INT NOT NULL default 0,\n" +
	"  newprize  INT NOT NULL default 0,\n" +
	"  prize1  INT NOT NULL default 0,\n" +
	"  prize2  INT NOT NULL default 0,\n" +
	"  prize3  INT NOT NULL default 0,\n" +
	"  enddate decimal(20,0) NOT NULL default 0,\n" +
	"  finished INT NOT NULL default 0,\n" +
	"  PRIMARY KEY (`id`,`idnr`)\n" +
	")"
