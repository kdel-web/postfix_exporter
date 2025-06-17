#!/usr/bin/env python3

import gzip
import re
import sys

from collections import Counter
from functools import cache
from pprint import pprint

"""
"""

@cache  # test this again # this is definitely still a thing. minor, but definitely
def get_regex(which: str):
    """
    choices are 'smtp_info_pattern', 'msg_id_pattern', 'email_addr_pattern'\n
    regex bank called by other funcs herein.
    """
    _smtp_info_pattern = re.compile(
        r".*?(postfix(-slow|-fast|-medium|-restrictive)?\/smtp)\[.*?:\s([A-F0-9]{6,}):\s.*?\<(.*?@.*?)\>,"
    )
    _msg_id_pattern = re.compile(r"\s([A-F0-9]{6,}):")
    _new_email_addr = re.compile(r"<(.*?@?.*?)>:")
    _cleanupline = re.compile(r"\s([A-F0-9]{6,}):.*?from=<(.*?@.*?)>.*?to=<(.*?@.*?)>")

    # NO _stricter_cleanupline = re.compile(r"\s([A-F0-9]{6,}):.*?(?:replace:\sheader\sMessage-Id:).*?from=<(.*?@.*?)>.*?to=<(.*?@.*?)>")
    _stricter_cleanupline = re.compile(
        r"\s([A-F0-9]{6,}):.*?(?:replace:\sheader\sReceived:).*?from=<(.*?@.*?)>.*?to=<(.*?@.*?)>"
    )
    __bank = {
        "smtp_info_pattern": _smtp_info_pattern,
        "msg_id_pattern": _msg_id_pattern,
        "email_addr_pattern": _new_email_addr,
        "cleanupline": _cleanupline,
        "newcleanupline": _stricter_cleanupline,
    }
    try:
        x = __bank[which]
    except KeyError:
        print(f"Error! Unknown selection: {which}")
        sys.exit(1)
    return x


def chkfile_zip(maillog: str) -> bool:
    """
    returns True if gz compressed file detected, else False.
    also handles both potential file not found and privilege errors.\n
    quits program on error.
    """
    try:
        with open(maillog, "rb") as z:
            zcontents = z.read(2)
        if zcontents == b"\x1f\x8b":
            return True
    except FileNotFoundError:
        print(f"Warning! {maillog = } | file not found")
        sys.exit(0)
    except PermissionError:
        print(f"Warning! {maillog = } | permission error encountered")
        sys.exit(0)
    return False


def gen_maillog_line(maillog: str = "/var/log/maillog"):
    """
    cheat codes
    """
    zipcheck = chkfile_zip(maillog=maillog)
    if zipcheck:
        with gzip.open(maillog, "rt") as f:
            for line in f:
                if (
                    not "postfix/discard" in line
                    and not "TLS connection established" in line
                    and not "opendkim" in line
                ):
                    yield line.strip()
    else:
        with open(maillog, "rt") as f:
            for line in f:
                if (
                    not "postfix/discard" in line
                    and not "TLS connection established" in line
                    and not "opendkim" in line
                ):
                    yield line.strip()


def main(
    logfile: str = "/var/log/maillog",
) -> dict:
    MsgCounter = Counter(
        {
            "accepted_in": 0,
            "cleanup": 0,
            "no_queue": 0,
            "bounced": 0,
            "deferred_tries": 0,
            "sent_msgs": 0,
        }
    )

    # like a queue, except there is not any type of order
    SET_running_qids = set()
    # also, should only contain queue ids that we don't have any
    # other info on.

    # no queues
    invalid_emails = []
    # NOTE right now these are being logged and not returned

    # representing accepted in
    # but contains destination (and sender) address
    SET_cleanup_records = set()  # tuple (qid, dest)

    bounced_records = []  # tuple (in qid, bounce msg qid)
    # NOTE: probably also want to know the email address and reason
    # for bounce

    deferred_records = []  # tuple (in qid, dest)
    # NOTE: number of times a record appears is how many times it was tried
    # NOTE: this to be made into a set for membership checking

    # deferred_qids = [] # to be made into a Counter
    # NOTE: no point in counting these separately probably.. can just parse the records above later

    sent_records = []  # tuple (qid, dest)

    master_recorder = dict()

    def runner():
        for line in gen_maillog_line(maillog=logfile):
            if "postfix/smtpd" in line and not "connect" in line:
                if "NOQUEUE:" in line:
                    m = get_regex("email_addr_pattern").search(line)
                    # invalid_emails.append(m.group(1))
                    MsgCounter["no_queue"] += 1
                else:
                    m = get_regex("msg_id_pattern").search(line)
                    if m:
                        SET_running_qids.add(m[1])
                        MsgCounter["accepted_in"] += 1
                        #master_recorder[m[1]] = {
                        #    "was_sent": False,
                        #    "was_defer": False,
                        #    "tries": 0,
                        #    "bounce": False,
                        #}
                # counting accepted in using cleanup potentially

            elif "postfix/cleanup" in line:
                m = get_regex("newcleanupline").search(line)
                if m:
                    MsgCounter["cleanup"] += 1
                    SET_running_qids.add(m[1])
                    SET_cleanup_records.add((m[1], m[3]))
                    master_recorder[(m[1], m[3])] = {
                        "was_sent": False,
                        "was_defer": False,
                        "tries": 0,
                        "bounce": False,
                    }

            elif "status=sent" in line:
                m = get_regex("smtp_info_pattern").search(line)
                if m:
                    MsgCounter["sent_msgs"] += 1
                    sent_records.append((m[3], m[4]))
                    SET_running_qids.discard(m[3])
                    if (m[3], m[4]) in master_recorder:
                        master_recorder[(m[3], m[4])]["was_sent"] = True
                    else:
                        master_recorder[(m[3], m[4])] = {
                            "was_sent": True,
                            "was_defer": None,
                            "tries": None,
                        }

            elif "status=deferred" in line:
                MsgCounter["deferred_tries"] += 1
                m = get_regex("smtp_info_pattern").search(line)
                if m:
                    SET_running_qids.discard(m[3])
                    deferred_records.append((m[3], m[4]))
                    if (m[3], m[4]) in master_recorder:
                        master_recorder[(m[3], m[4])]["was_defer"] = True
                        master_recorder[(m[3], m[4])]["tries"] += 1
                    else:
                        master_recorder[(m[3], m[4])] = {
                            "was_sent": None,
                            "was_defer": True,
                            "tries": 1,
                        }

            elif "postfix/bounce" in line:
                MsgCounter["bounced"] += 1
                m = get_regex("msg_id_pattern").findall(line)
                if m:
                    bounced_records.append(m)
                    SET_running_qids.discard(m[0])
                    ## this one needs adjusted or something
                    # not even getting the email addr bro
                    # go to bed
            # else:
            #    print(line)

        return

    runner()

    # dedupe_deferred_qids = {x for x in deferred_qids}

    if len(SET_running_qids) > 0:  # NOTE: this is a set #
        # actually no point here, already discarding from running
        # unknown_qids = running_qids - dedupe_deferred_qids
        print(f"Running Queue Ids: {len(SET_running_qids)}")
        # print(f"Comparing unknown Queue Ids: {len(unknown_qids)}") # this should be same as running anyway

    # parsing / reformatting of some data
    sent_domains = [d[1].split("@")[-1].lower().strip() for d in sent_records]

    SET_deferred_records = set(deferred_records)
    deferred_qids = [d[0] for d in deferred_records]

    count_deferred_tries = Counter(deferred_qids)
    count_individ_defers = Counter(SET_deferred_records)
    count_sent_domains = Counter(sent_domains)

    SET_cleanup_qids = {s[0] for s in SET_cleanup_records}
    SET_bounce_qids = {b[0] for b in bounced_records}
    SET_sent_qids = {c[0] for c in sent_records}
    SET_deferred_qids = {d[0] for d in SET_deferred_records}

    inbound_to_sent = SET_cleanup_qids & SET_sent_qids
    inbound_to_defer = SET_cleanup_qids & SET_deferred_qids
    inbound_to_bounce = SET_cleanup_qids & SET_bounce_qids

    main_out = {
        "running_qids": SET_running_qids,
        "cleanup_records": SET_cleanup_records,
        "sent_records": sent_records,
        "sent_domains": count_sent_domains,
        "bounced_records": bounced_records,
        # "deferred_records": deferred_records, # will include duplicates
        "deferred_records": SET_deferred_records,  # to check for membership
        "deferred_tries": count_deferred_tries,
        "deferred_deduped": count_individ_defers,
        # "dedupe_defer": dedupe_deferred_qids, # removing
        "MsgCounter": MsgCounter,
        "mr": master_recorder,
    }
    # save_state = ChainMap(running_qids, cleanup_records, sent_records, bounced_records, deferred_records, count_deferred_tries)

    pprint(MsgCounter)
    # pprint(count_deferred_tries)
    print(f"Count individual defers: {len(count_individ_defers)}")
    print(f"Inbound to sent: {len(inbound_to_sent) or None}")
    print(f"Inbound to defer: {len(inbound_to_defer) or None}")
    print(f"Inbound to bounce: {len(inbound_to_bounce) or None}")

    return main_out

#test_file = "/home/bobx/github/postfix_reporting/outputs/CP.201.wpn.maillog_27"
#test_file2 = "/home/bobx/github/postfix_reporting/outputs/202.maillog-20250314.gz"

if __name__ == "__main__":
    import argparse

    parser = argparse.ArgumentParser()
    parser.add_argument(
        "--search-file", help="Override search file", default="/var/log/maillog"
    )

    args = parser.parse_args()

    # hour = 10
    # quarter = 5
    # search_log = "/var/log/maillog"
    # save_state_file = "/home/kdellinger/_running_msgs_in"

    #test_file = "/home/bobx/github/postfix_reporting/outputs/CP.201.wpn.maillog_27"
    #test_file2 = "/home/bobx/github/postfix_reporting/outputs/202.maillog-20250314.gz"
    # test_outputs = Path("/home/kdellinger/onedrive/python/postfix_reporting/outputs")
    ##
    # test_205_28 = test_outputs / "205.full.20250328.gz"
    # maybe write two files: 1. known/final 2. running ?
    # test_save_state = "/home/bobx/github/postfix_reporting/_pros/_outs/test1"

    save_state = main(logfile=args.search_file)

    ## NOTE: confirmed this logic is returning similar results to the counter I'm running on flyer sends

    # target for CP.201.wpn.maillog_27
    # in: 14706
    # sent: 10933
    # deferred: 344 # deduped: 334
    # bounced: 160
    # no_queue: 30
    # UNKNOWNS --> 3720

    ### new test file: 205.full.20250328.gz
    # compared targets against flog_cutup_test.py
    # hour 9 quarter all (5)
    # in: 28191
    # sent: 25923
    # bounced: 368
    # deferred: 3954
    # no_queues: 54
    # deferred_retries: 961
    # inbound to bounce: 366
    ### confirmed also matches spreadsheet (as expected)

    ## confirmed matches numbers from this using MsgCounter counting
    ## inbound messages. switching to stricter regex to use count
    ## from cleanup instead.
    ## regex try two seems to have worked for hour 9. using:
    #### "replace:\sheader:\sReceived:" in non-capture group
    ## try hour 10
    # inbound is 45255** target total from below minus 4
    # these are the four:
    # so why didn't they match
    # 413E320576B0
    # B659D206F16A
    # 8CBED20436BC
    # B24B720436BC
    # for those 4, the only log line in smtpd. there are no other lines
    # with those queue ids in them... which seems interesting
    # similarly, they are the only 4 lines in graylog too...

    # no queues and bounces looking good though

    ### 205.full.20250328.gz
    # compared targets against flog_cutup_test.py
    # hour 10 quarter all
    # in: 45259
    # sent: 39793
    # bounced: 678
    # deferred: 5086
    # no_queues: 106
    # deferred_retries: 2348
    # inbound to bounce: 669
    # confirmed match spreadsheet
    ####

    # hour 11 targets
    # in: 51509
    # sent: 47486
    # bounced: 619
    # deferred: 6425
    # no_queues: 80
    # deferred_retries: 2540
    # inbound to bounce: 609
