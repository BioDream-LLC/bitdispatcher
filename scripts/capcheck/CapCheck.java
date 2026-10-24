// CapCheck validates one CAP message with Google's CAP library (Apache-2.0): the OASIS CAP 1.2 schema
// and rules, then the CAP-v1.2 USA IPAWS Profile v1.0 as the library implements it. It prints one line
// per finding, "ERROR", "WARNING" or "RECOMMENDATION", the XPath and the message, and exits 1 when there
// is an error. Built and run by scripts/cap-validator.sh; nothing of it is in bitdispatcher itself.
import com.google.publicalerts.cap.Alert;
import com.google.publicalerts.cap.CapXmlParser;
import com.google.publicalerts.cap.Reason;
import com.google.publicalerts.cap.Reasons;
import com.google.publicalerts.cap.profile.us.Ipaws1Profile;
import java.nio.charset.StandardCharsets;
import java.nio.file.Files;
import java.nio.file.Paths;

public class CapCheck {
  public static void main(String[] args) throws Exception {
    String xml = new String(Files.readAllBytes(Paths.get(args[0])), StandardCharsets.UTF_8);
    Reasons.Builder parse = Reasons.newBuilder();
    Alert alert = new CapXmlParser(true, true).parseFrom(xml, parse);
    boolean bad = print("cap", parse.build());
    bad |= print("ipaws", new Ipaws1Profile(true).validate(alert));
    System.out.println("checked " + alert.getIdentifier());
    System.exit(bad ? 1 : 0);
  }

  static boolean print(String what, Reasons rs) {
    boolean bad = false;
    for (Reason r : rs) {
      bad |= r.getLevel() == Reason.Level.ERROR;
      System.out.println(r.getLevel() + " " + what + " " + r.getXPath() + " " + r.getMessage());
    }
    return bad;
  }
}
