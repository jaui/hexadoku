#include <iostream>
#include <vector>
#include <algorithm>
#include <memory>
using namespace std;

class Possible {
   vector<bool> _b;
public:
   Possible() : _b(16, true) {}
   bool   is_on(int i) const { return _b[i]; }
   int    count()      const { return std::count(_b.begin(), _b.end(), true); }
   void   eliminate(int i)   { _b[i] = false; }
   int    val()        const {
      auto it = find(_b.begin(), _b.end(), true);
      return (it != _b.end() ? (it - _b.begin()) : -1);
   }
   string str(int wth) const;
};

string Possible::str(int width) const {
   string s(width, ' ');
   int k = 0;
   for (int i = 0; i <= 15; i++) {
      if (is_on(i)) s[k++] = i<10 ? '0'+i:'A'+i-10;
   }
   return s;
}

class Hexdoku {
   vector<Possible> _cells;
   static vector< vector<int> > _group, _neighbors, _groups_of;

   bool     eliminate(int k, int val);
public:
   Hexdoku(string s);
   static void init();

   Possible possible(int k) const { return _cells[k]; }
   bool     is_solved() const;
   bool     assign(int k, int val);
   int      least_count() const;
   void     write(ostream& o) const;
};

bool Hexdoku::is_solved() const {
   for (unsigned int k = 0; k < _cells.size(); k++) {
      if (_cells[k].count() != 1) {
         return false;
      }
   }
   return true;
}

void Hexdoku::write(ostream& o) const {
   int width = 1;
   for (unsigned int k = 0; k < _cells.size(); k++) {
      width = max(width, 1 + _cells[k].count());
   }
   const string sep(4 * width, '-');
   for (int i = 0; i < 16; i++) {
      if (i == 4 || i == 8 || i == 12) {
         o << sep << "+-" << sep << "+-" << sep << "+" << sep << endl;
      }
      for (int j = 0; j < 16; j++) {
         if (j == 4 || j == 8 || j == 12) o << "| ";
         o << _cells[i*16 + j].str(width);
      }
      o << endl;
   }
}

vector< vector<int> >
Hexdoku::_group(64), Hexdoku::_neighbors(256), Hexdoku::_groups_of(256);

void Hexdoku::init() {
   for (int i = 0; i < 16; i++) {
      for (int j = 0; j < 16; j++) {
         const int k = i*16 + j;
         const int x[3] = {i, 16 + j, 32 + (i/4)*4 + j/4};
         for (int g = 0; g < 3; g++) {
            _group[x[g]].push_back(k);
            _groups_of[k].push_back(x[g]);
         }
      }
   }
   for (unsigned int k = 0; k < _neighbors.size(); k++) {
      for (unsigned int x = 0; x < _groups_of[k].size(); x++) {
         for (int j = 0; j < 16; j++) {
            unsigned int k2 = _group[_groups_of[k][x]][j];
            if (k2 != k) _neighbors[k].push_back(k2);
         }
      }
   }
}

bool Hexdoku::assign(int k, int val) {
   for (int i = 0; i < 16; i++) {
      if (i != val) {
         if (!eliminate(k, i)) return false;
      }
   }
   return true;
}

bool Hexdoku::eliminate(int k, int val) {
   if (!_cells[k].is_on(val)) {
      return true;
   }
   _cells[k].eliminate(val);
   const int N = _cells[k].count();
   if (N == 0) {
      return false;
   } else if (N == 1) {
      const int v = _cells[k].val();
      for (unsigned int i = 0; i < _neighbors[k].size(); i++) {
         if (!eliminate(_neighbors[k][i], v)) return false;
      }
   }
   for (unsigned int i = 0; i < _groups_of[k].size(); i++) {
      const int x = _groups_of[k][i];
      int n = 0, ks;
      for (int j = 0; j < 16; j++) {
         const int p = _group[x][j];
         if (_cells[p].is_on(val)) {
            n++, ks = p;
         }
      }
      if (n == 0) {
         return false;
      } else if (n == 1) {
         if (!assign(ks, val)) {
            return false;
         }
      }
   }
   return true;
}

int Hexdoku::least_count() const {
   int k = -1, min;
   for (unsigned int i = 0; i < _cells.size(); i++) {
      const int m = _cells[i].count();
      if (m > 1 && (k == -1 || m < min)) {
         min = m, k = i;
      }
   }
   return k;
}

Hexdoku::Hexdoku(string s)
  : _cells(256)
{
   int k = 0;
   int skip=0;
   for (unsigned int i = 0; i < s.size() ; i++) {
	 if (k<256) {
       if (s[i] >= '0' && s[i] <= '9') {
         if (!assign(k, s[i] - '0')) {
            cerr << "error" << endl;
            return;
         }
         k++;
       } else if (s[i] >= 'a' && s[i] <= 'f') {
		  if (!assign(k, s[i] + 10 - 'a')) {
			 cerr << "error" << endl;
			 return;
		  }
		  k++;
       } else if (s[i] >= 'A' && s[i] <= 'F') {
		  if (!assign(k, s[i] + 10 - 'A')) {
			 cerr << "error" << endl;
			 return;
		  }
		  k++;
       } else if (s[i] == '.') {
          k++;
       }
	 } else {
		 skip++;
	 }
   }
   if (skip>0) {
	   cerr << "skipped "<<skip<<" chars. max 16*16=256 chars [0-9a-fA-F]=val [.]=empty"<<endl;
   }
}

unique_ptr<Hexdoku> solve(unique_ptr<Hexdoku> S) {
   if (S == nullptr || S->is_solved()) {
      return S;
   }
   int k = S->least_count();
   Possible p = S->possible(k);
   for (int i = 0; i <= 15; i++) {
      if (p.is_on(i)) {
         unique_ptr<Hexdoku> S1(new Hexdoku(*S));
         if (S1->assign(k, i)) {
            if (auto S2 = solve(std::move(S1))) {
               return S2;
            }
         }
      }
   }
   return {};
}

int main() {
  Hexdoku::init();
  string line;
  line="E......A...6.F8..65...E.18.F.0.A3.7B..65.D...2..8.....B..34...5....07...9.....2..B...2.0F7.8D..65.E...FBD16.C....D.6.3..2.0..A.7....D.....C.287.73BE..9C0A82..6...A..1....7E.B9.29....0.64D...A.476..F.....A0..2B.C3A.5480...EF.DE9.0C2.4.F5..18F..5.B....19.4..";
  //while (getline(cin, line)) {
  //cout << line << endl;
    if (auto SI=new Hexdoku(line)) {
      SI->write(cout);

	  if (auto SO = solve(unique_ptr<Hexdoku>(SI))) {
         SO->write(cout);
      }
	  else {
         cout << "No solution";
      }
      cout << endl;
   }
  //}
}
